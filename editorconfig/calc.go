package editorconfig

import (
	"encoding/json"
	"fmt"

	"github.com/editorconfig/editorconfig-core-go/v2"
	"github.com/mrvnmyr/oat/common"
)

func Calc(path string) {
	def, err := editorconfig.GetDefinitionForFilename(path)
	common.Check(err)

	{ // print as JSON
		fooOutput, err := json.Marshal(&def)
		common.Check(err)
		fmt.Printf("%s\n", fooOutput)
	}
}
