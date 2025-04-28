package verz

import "fmt"

// Set with `-ldflags "-X github.com/prequel-dev/preq/internal/pkg/verz.Githash=..."`
// Set with `-ldflags "-X github.com/prequel-dev/preq/internal/pkg/verz.Major=..."`
// Set with `-ldflags "-X github.com/prequel-dev/preq/internal/pkg/verz.Minor=..."`
// Set with `-ldflags "-X github.com/prequel-dev/preq/internal/pkg/verz.Build=..."`
// Set with `-ldflags "-X github.com/prequel-dev/preq/internal/pkg/verz.Date=..."`
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
