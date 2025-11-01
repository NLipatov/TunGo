//go:build windows

package route

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"tungo/infrastructure/PAL"
)

type v6Wrapper struct {
	commander PAL.Commander
}

func newV6Wrapper(c PAL.Commander) Contract { return &v6Wrapper{commander: c} }

// DefaultRoute parses `route print -6` lines that contain `::/0`.
// Heuristic (locale-agnostic):
//   - metric = last integer token *to the left* of "::/0"
//   - idx    = first integer token *to the right* of "::/0"
//   - gw     = first IPv6 literal token *to the right* of idx
//   - ifName = tail after gw; if empty, resolve via InterfaceByIndex(idx)
func (w *v6Wrapper) DefaultRoute() (gw, ifName string, metric int, err error) {
	out, execErr := w.commander.CombinedOutput("route", "print", "-6")
	if execErr != nil {
		return "", "", 0, fmt.Errorf("route print -6: %w", execErr)
	}
	best := int(^uint(0) >> 1)
	var bestGW, bestIf string

	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := sc.Text()
		pos := strings.Index(line, "::/0")
		if pos < 0 {
			continue
		}
		left := strings.TrimSpace(line[:pos])
		right := strings.TrimSpace(line[pos+len("::/0"):])

		met := lastInt(left)
		if met < 0 {
			met = 1 << 30
		}
		rTokens := strings.Fields(right)
		if len(rTokens) == 0 {
			continue
		}
		idx := -1
		gwTokPos := -1
		// idx = first int on the right
		for i, t := range rTokens {
			if v, e := strconv.Atoi(t); e == nil {
				idx = v
				// gateway = first IPv6 after this index token
				for j := i + 1; j < len(rTokens); j++ {
					ip := parseIPv6(rTokens[j])
					if ip != "" {
						gwTokPos = j
						gw = ip
						break
					}
				}
				break
			}
		}
		if gwTokPos == -1 {
			for j := 0; j < len(rTokens); j++ {
				ip := parseIPv6(rTokens[j])
				if ip != "" {
					gwTokPos = j
					gw = ip
					break
				}
			}
		}
		if gw == "" {
			continue
		}
		// Interface name is everything after gw token
		ifName = strings.TrimSpace(strings.Join(rTokens[gwTokPos+1:], " "))
		if ifName == "" && idx > 0 {
			iface, _ := net.InterfaceByIndex(idx)
			if iface != nil {
				ifName = iface.Name
			}
		}
		if ifName == "" {
			continue
		}
		if met < best {
			best = met
			bestGW = gw
			bestIf = ifName
		}
	}
	if bestGW == "" || bestIf == "" {
		return "", "", 0, errors.New("default v6 route not found")
	}
	return bestGW, bestIf, best, nil
}

func (w *v6Wrapper) Delete(dst string) error {
	dst = strings.TrimSpace(dropZone(dst))
	if dst == "" {
		return fmt.Errorf("route -6 delete: empty destination")
	}
	out, err := w.commander.CombinedOutput("route", "-6", "delete", dst)
	if err != nil {
		return fmt.Errorf("route delete %s: %v, output: %s", dst, err, out)
	}
	return nil
}

func (w *v6Wrapper) Print(t string) ([]byte, error) {
	args := []string{"print", "-6"}
	if s := strings.TrimSpace(t); s != "" {
		args = append(args, dropZone(s))
	}
	out, err := w.commander.CombinedOutput("route", args...)
	if err != nil {
		return nil, fmt.Errorf("route %s: %v, output: %s", strings.Join(args, " "), err, out)
	}
	return out, nil
}

// helpers

func parseIPv6(tok string) string {
	ip := net.ParseIP(dropZone(tok))
	if ip == nil || ip.To4() != nil {
		return ""
	}
	return ip.String()
}

func lastInt(s string) int {
	best := -1
	for _, t := range strings.Fields(s) {
		if v, e := strconv.Atoi(t); e == nil {
			best = v
		}
	}
	return best
}

func dropZone(s string) string {
	if i := strings.IndexByte(s, '%'); i >= 0 {
		return s[:i]
	}
	return s
}
