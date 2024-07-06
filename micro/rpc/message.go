package rpc

type Request struct {
	ServiceName string
	MethodName  string
	Args        []byte
}

type Response struct {
	Data []byte
}
