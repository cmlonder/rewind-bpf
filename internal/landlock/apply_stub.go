//go:build !linux

package landlock

import "fmt"

func Apply(Plan) error {
	return fmt.Errorf("Landlock is only available on Linux")
}
