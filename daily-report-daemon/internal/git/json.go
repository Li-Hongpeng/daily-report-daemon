package git

import "encoding/json"

func marshalActivity(act *Activity) ([]byte, error) {
	return json.MarshalIndent(act, "", "  ")
}
