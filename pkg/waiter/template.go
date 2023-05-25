package waiter

import (
	"os"
)

func execTemplate(value any, mapping func(string) string) any {
	if value == nil {
		return nil
	}

	switch x := value.(type) {
	case map[string]any:
		return execMapTemplate(x, mapping)

	case string:
		return os.Expand(x, mapping)

	case []any:
		var res []any
		for _, y := range x {
			res = append(res, execTemplate(y, mapping))
		}
		return res

	default:
	}

	return value
}

func execMapTemplate(input map[string]any, mapping func(string) string) map[string]any {
	if input == nil {
		return nil
	}

	out := map[string]any{}
	for key, value := range input {
		out[key] = execTemplate(value, mapping)
	}

	return out
}
