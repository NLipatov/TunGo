package splits

func FullTunnelV4() []string {
	return []string{"0.0.0.0/1", "128.0.0.0/1"}
}
func FullTunnelV6() []string {
	return []string{"::/1", "8000::/1"}
}
