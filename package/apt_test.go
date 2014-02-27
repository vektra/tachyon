package apt

import (
	"fmt"
	"github.com/vektra/tachyon"
	"testing"
)

func TestApt(t *testing.T) {
	res, err := tachyon.RunAdhocTask("apt", "pkg=s3cmd dryrun=true")

	fmt.Printf("%#v, %#v\n", res, err)
}
