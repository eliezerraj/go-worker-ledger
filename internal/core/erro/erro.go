package erro

import (
	"errors"
)

var (
	ErrNotFound 		= errors.New("item not found")
	ErrUpdate			= errors.New("update unsuccessful")
	ErrUpdateRows		= errors.New("update affect 0 rows")
	ErrUnmarshal 		= errors.New("unmarshal json error")
	ErrUnauthorized 	= errors.New("not authorized")
	ErrServer		 	= errors.New("server identified error")
	ErrHTTPForbiden		= errors.New("forbiden request")
	ErrInvalid			= errors.New("invalid data")
)