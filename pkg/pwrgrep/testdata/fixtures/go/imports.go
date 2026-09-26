package fixture

import (
	// ruleid: go-imports
	"fmt"
	// ruleid: go-imports
	str "strings"
)

// ruleid: go-imports
import "os"

// ok: go-imports
var s = "os"

func f() { fmt.Println(str.ToUpper(s), os.Args) }
