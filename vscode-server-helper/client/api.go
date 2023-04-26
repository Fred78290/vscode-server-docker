package client

type ErrorObject struct {
	Code   int
	Reason string
}

type ReadyObject struct {
	Codespace string
	Ready     bool
}

type ReadyResponse struct {
	Status int
	Result ReadyObject
}

type DeletedObject struct {
	Codespace string
	Deleted   bool
}

type DeletedResponse struct {
	Status int
	Result DeletedObject
}

type ExistsObject struct {
	Codespace string
	Exists    bool
}

type ExistsResponse struct {
	Status int
	Result ExistsObject
}

type ErrorResponse struct {
	Status int
	Error  ErrorObject
}
