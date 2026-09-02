package terminal

import (
	"os"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

var _ = sftp.Client{}
var _ = ssh.Client{}

func listLocalDir(path string) ([]map[string]any, error) {
	if path == "" {
		path = "."
	}
	ents, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ents))
	for _, e := range ents {
		info, _ := e.Info()
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		out = append(out, map[string]any{
			"name":  e.Name(),
			"isDir": e.IsDir(),
			"size":  size,
		})
	}
	return out, nil
}
