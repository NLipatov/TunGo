package shutdown

import (
	"os"
	"os/signal"
)

type Notifier struct {
}

func NewNotifier() *Notifier {
	return &Notifier{}
}

func (s *Notifier) Notify(c chan<- os.Signal, sig ...os.Signal) {
	signal.Notify(c, sig...)
}
func (s *Notifier) Stop(c chan<- os.Signal) {
	signal.Stop(c)
}
