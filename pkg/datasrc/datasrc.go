package datasrc

type DataSrc interface {
	Type() string
	Meta() map[string]string
}
