package run

import "github.com/cjxpj/nebula/dto"

// build
type Build struct {
	G_v   *dto.Val
	V     *dto.Val
	Path  string
	Uid   string
	Cache bool
}
