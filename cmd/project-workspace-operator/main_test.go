package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_NewProjectWorkspaceOperatorCommand(t *testing.T) {
	cmd := NewProjectWorkspaceOperatorCommand()
	assert.NotEmpty(t, cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotNil(t, cmd.RunE)
	assert.True(t, cmd.HasSubCommands())

	cmds := cmd.Commands()
	assert.Len(t, cmds, 3)

	cmdNames := []string{}
	for _, c := range cmds {
		cmdNames = append(cmdNames, c.Name())
	}

	assert.Contains(t, cmdNames, "start")
	assert.Contains(t, cmdNames, "init")
}

func Test_Options_Complete(t *testing.T) {
	testCases := []struct {
		desc             string
		kubeconfigPath   string
		crateClusterPath string
		expected         error
	}{
		{
			desc:             "should load kubeconfig when specified directly",
			kubeconfigPath:   "testdata/kubeconfig",
			crateClusterPath: "testdata/kubeconfig",
		},
		{
			desc:             "should load kubeconfig when present in directory",
			kubeconfigPath:   "testdata/kubeconfig",
			crateClusterPath: "testdata",
		},
		{
			desc:             "should build rest.Config when using OIDC trust",
			kubeconfigPath:   "testdata/kubeconfig",
			crateClusterPath: "testdata/oidc",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			o := &Options{
				rawOptions: rawOptions{
					CrateClusterPath: tC.crateClusterPath,
				},
			}
			os.Setenv("KUBECONFIG", tC.kubeconfigPath)
			err := o.Complete()
			assert.Equal(t, tC.expected, err)
		})
	}
}
