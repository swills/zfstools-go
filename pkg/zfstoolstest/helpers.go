package zfstoolstest

import (
	"os"

	exec "golang.org/x/sys/execabs"
)

// IsTestEnv returns the env is in testing or not
func IsTestEnv() bool {
	return os.Getenv("GO_WANT_HELPER_PROCESS") == "1"
}

// MakeFakeCommand returns the fake exec.Command() function for testing
func MakeFakeCommand(mockFuncName string) func(command string, args ...string) *exec.Cmd {
	return func(command string, args ...string) *exec.Cmd {
		mockArg := "-test.run=" + mockFuncName
		cs := append([]string{mockArg, "--", command}, args...) // -test.run means the self mock function
		cmd := exec.Command(os.Args[0], cs...)                  //nolint:gosec
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "GO_WANT_HELPER_PROCESS=1")

		return cmd
	}
}
