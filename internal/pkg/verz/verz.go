package verz

import "fmt"

var (
	Githash string
	Major   string
	Minor   string
	Build   string
	Date    string
)

func Semver() string {
	return fmt.Sprintf("%s.%s.%s", Major, Minor, Build)
}
