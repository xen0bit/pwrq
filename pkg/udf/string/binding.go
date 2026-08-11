package string

import (
	"fmt"

	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// strFromPipeline resolves a string from the pipeline value, naming the cmdlet
// in any error so the message points at the call the user wrote.
func strFromPipeline(v any, name string) (string, error) {
	input, err := stringInput(common.BindValue(v))
	if err != nil {
		return "", fmt.Errorf("%s: %v", name, err)
	}
	return input, nil
}
