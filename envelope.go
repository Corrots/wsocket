package wsocket

// 信封
type envelope struct {
	t      int
	msg    []byte
	filter filterFunc
}
