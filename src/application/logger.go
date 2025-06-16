package application

type Logger interface {
	Printf(format string, v ...any)
}
