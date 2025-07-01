package editorconfig

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/editorconfig/editorconfig-core-go/v2"
	"github.com/mrvnmyr/oat/common"
)

func Calc(path string, key string) {
	def, err := editorconfig.GetDefinitionForFilename(path)
	common.Check(err)

	{ // print as JSON
		if key != "" {
			result := make(map[string]interface{})

			jsonOutput, err := json.Marshal(def)
			common.Check(err)

			err = json.Unmarshal(jsonOutput, &result)
			common.Check(err)

			if val, ok := result[key]; ok {
				fmt.Printf("%s\n", val)
			} else {
				// fmt.Fprintf(os.Stderr, "Unknown key: %s\n", key)
				os.Exit(1)
			}
		} else {
			jsonOutput, err := json.Marshal(&def)
			common.Check(err)
			fmt.Printf("%s\n", jsonOutput)
		}
	}
}
