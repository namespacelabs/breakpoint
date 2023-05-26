package jsonfile

import (
	"encoding/json"
	"os"
)

func Load(filename string, target any) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}

	return json.NewDecoder(f).Decode(target)
}
