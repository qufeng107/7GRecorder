package version

import "fmt"

var BuildSHA = "dev"

func Info() string {
	return fmt.Sprintf("7grecorder %s", BuildSHA)
}
