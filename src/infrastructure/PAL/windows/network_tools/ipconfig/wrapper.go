package ipconfig

import (
	"fmt"
	"tungo/infrastructure/PAL"
)

type Wrapper struct {
	commander PAL.Commander
}

func NewWrapper(commander PAL.Commander) Contract {
	return &Wrapper{
		commander: commander,
	}
}

func (w *Wrapper) FlushDNS() error {
	output, err := w.commander.CombinedOutput("ipconfig", "/flushdns")
	if err != nil {
		return fmt.Errorf("flushdns: %s", output)
	}
	return nil
}
