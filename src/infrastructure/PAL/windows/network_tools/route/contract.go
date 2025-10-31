package route

type Contract interface {
	RouteDelete(destinationIP string) error
}
