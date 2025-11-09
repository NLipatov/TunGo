//go:build windows

package nif

import (
	"fmt"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

type resolver struct {
	cacheMu sync.RWMutex
	cache   map[string]winipcfg.LUID
}

func NewResolver() Contract {
	return &resolver{
		cache: make(map[string]winipcfg.LUID),
	}
}

func (r *resolver) NetworkInterfaceByName(ifName string) (winipcfg.LUID, error) {
	if v, ok := r.getCached(ifName); ok {
		return v, nil
	}
	want := strings.TrimSpace(ifName)
	if want == "" {
		return 0, fmt.Errorf("empty interface name")
	}

	addrs, err := winipcfg.GetAdaptersAddresses(winipcfg.AddressFamily(windows.AF_UNSPEC), 0)
	if err != nil {
		return 0, err
	}

	type cand struct {
		luid  winipcfg.LUID
		up    bool
		label string
		name  string
	}

	pick := func(cs []cand) (winipcfg.LUID, bool, bool) {
		switch len(cs) {
		case 0:
			return 0, false, false
		case 1:
			return cs[0].luid, true, false
		default:
			var ups []cand
			for _, c := range cs {
				if c.up {
					ups = append(ups, c)
				}
			}
			if len(ups) == 1 {
				return ups[0].luid, true, false
			}
			if len(ups) > 1 {
				var best winipcfg.LUID
				bestIdx := ^uint32(0)
				for _, c := range ups {
					if ifRow, _ := c.luid.Interface(); ifRow != nil && ifRow.InterfaceIndex < bestIdx {
						bestIdx, best = ifRow.InterfaceIndex, c.luid
					}
				}
				return best, true, false
			}
			return 0, false, true // got matches, but none Up
		}
	}

	pass := func(match func(a *winipcfg.IPAdapterAddresses) (bool, cand)) (winipcfg.LUID, bool, bool) {
		var cs []cand
		for _, a := range addrs {
			if ok, c := match(a); ok {
				if ifRow, _ := a.LUID.Interface(); ifRow != nil && ifRow.OperStatus == winipcfg.IfOperStatusUp {
					c.up = true
				}
				cs = append(cs, c)
			}
		}
		luid, ok, amb := pick(cs)
		return luid, ok, amb
	}

	// 1) FriendlyName
	if luid, ok, allMatchesDown := pass(func(a *winipcfg.IPAdapterAddresses) (bool, cand) {
		if r.matchName(a.FriendlyName(), want) {
			return true, cand{luid: a.LUID, label: "FriendlyName", name: a.FriendlyName()}
		}
		return false, cand{}
	}); ok {
		r.putCached(ifName, luid)
		return luid, nil
	} else if allMatchesDown {
		return 0, fmt.Errorf("found matches for %q by FriendlyName, but all are down", ifName)
	}

	// 2) Alias (MIB)
	if luid, ok, allMatchesDown := pass(func(a *winipcfg.IPAdapterAddresses) (bool, cand) {
		if ifRow, _ := a.LUID.Interface(); ifRow != nil && r.matchName(ifRow.Alias(), want) {
			return true, cand{luid: a.LUID, label: "Alias", name: ifRow.Alias()}
		}
		return false, cand{}
	}); ok {
		r.putCached(ifName, luid)
		return luid, nil
	} else if allMatchesDown {
		return 0, fmt.Errorf("found matches for %q by Alias, but all are down", ifName)
	}

	// 3) AdapterName
	if luid, ok, allMatchesDown := pass(func(a *winipcfg.IPAdapterAddresses) (bool, cand) {
		if r.matchName(a.AdapterName(), want) {
			return true, cand{luid: a.LUID, label: "AdapterName", name: a.AdapterName()}
		}
		return false, cand{}
	}); ok {
		r.putCached(ifName, luid)
		return luid, nil
	} else if allMatchesDown {
		return 0, fmt.Errorf("found matches for %q by AdapterName, but all are down", ifName)
	}

	// 4) Description
	if luid, ok, allMatchesDown := pass(func(a *winipcfg.IPAdapterAddresses) (bool, cand) {
		if r.matchName(a.Description(), want) {
			return true, cand{luid: a.LUID, label: "Description(IPAA)", name: a.Description()}
		}
		if ifRow, _ := a.LUID.Interface(); ifRow != nil && r.matchName(ifRow.Description(), want) {
			return true, cand{luid: a.LUID, label: "Description(MIB)", name: ifRow.Description()}
		}
		return false, cand{}
	}); ok {
		r.putCached(ifName, luid)
		return luid, nil
	} else if allMatchesDown {
		return 0, fmt.Errorf("found matches for %q by Description, but all are down", ifName)
	}
	return 0, fmt.Errorf("interface %q not found", ifName)
}

// matchName compares Windows adapter names case-insensitively and trims spaces.
func (r *resolver) matchName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func (r *resolver) NetworkInterfaceName(luid winipcfg.LUID) string {
	if addrs, err := winipcfg.GetAdaptersAddresses(winipcfg.AddressFamily(windows.AF_UNSPEC), 0); err == nil {
		for _, a := range addrs {
			if a.LUID == luid {
				if s := strings.TrimSpace(a.FriendlyName()); s != "" {
					return s
				}
				break
			}
		}
	}
	if ifRow, _ := luid.Interface(); ifRow != nil {
		if s := strings.TrimSpace(ifRow.Alias()); s != "" {
			return s
		}
		if s := strings.TrimSpace(ifRow.Description()); s != "" {
			return s
		}
	}
	return ""
}

func (r *resolver) getCached(ifName string) (winipcfg.LUID, bool) {
	key := r.canonKey(ifName)
	r.cacheMu.RLock()
	v, ok := r.cache[key]
	r.cacheMu.RUnlock()
	if !ok {
		return 0, false
	}
	// validate v, if still valid - return it
	if ifRow, err := v.Interface(); err == nil && ifRow != nil {
		return v, true
	}
	// if not valid - remove it
	r.cacheMu.Lock()
	delete(r.cache, key)
	r.cacheMu.Unlock()
	return 0, false
}

func (r *resolver) putCached(ifName string, luid winipcfg.LUID) {
	r.cacheMu.Lock()
	r.cache[r.canonKey(ifName)] = luid
	r.cacheMu.Unlock()
}
func (r *resolver) canonKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
