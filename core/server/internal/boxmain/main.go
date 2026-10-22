package boxmain

import (
	"context"
	"nekobox_core/internal/coreinclude"
	"os"
	"os/user"
	"strconv"

	"github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/experimental/deprecated"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing/service"
	"github.com/sagernet/sing/service/filemanager"

	"github.com/spf13/cobra"
)

var (
	globalCtx         context.Context
	configPaths       []string
	configDirectories []string
	workingDir        string
	disableColor      bool
)

var mainCommand = &cobra.Command{
	Use:              "sing-box",
	PersistentPreRun: preRun,
}

func init() {
	mainCommand.PersistentFlags().StringArrayVarP(&configPaths, "config", "c", nil, "set configuration file path")
	mainCommand.PersistentFlags().StringArrayVarP(&configDirectories, "config-directory", "C", nil, "set configuration directory path")
	mainCommand.PersistentFlags().StringVarP(&workingDir, "directory", "D", "", "set working directory")
	mainCommand.PersistentFlags().BoolVarP(&disableColor, "disable-color", "", false, "disable color output")
}

func preRun(cmd *cobra.Command, args []string) {
	globalCtx = newServiceContext()
	if disableColor {
		DisableColor()
	}
	if workingDir != "" {
		if _, err := os.Stat(workingDir); err != nil {
			filemanager.MkdirAll(globalCtx, workingDir, 0o777)
		}
		if err := os.Chdir(workingDir); err != nil {
			log.Fatal(err)
		}
	}
	if len(configPaths) == 0 && len(configDirectories) == 0 {
		configPaths = append(configPaths, "config.json")
	}
}

// Each RPC-created instance needs its own registry/context. Concurrent tests
// must not reset the active profile's global context or change process cwd.
func newServiceContext() context.Context {
	ctx := context.Background()
	sudoUser := os.Getenv("SUDO_USER")
	sudoUID, _ := strconv.Atoi(os.Getenv("SUDO_UID"))
	sudoGID, _ := strconv.Atoi(os.Getenv("SUDO_GID"))
	if sudoUID == 0 && sudoGID == 0 && sudoUser != "" {
		sudoUserObject, _ := user.Lookup(sudoUser)
		if sudoUserObject != nil {
			sudoUID, _ = strconv.Atoi(sudoUserObject.Uid)
			sudoGID, _ = strconv.Atoi(sudoUserObject.Gid)
		}
	}
	if sudoUID > 0 && sudoGID > 0 {
		ctx = filemanager.WithDefault(ctx, "", "", sudoUID, sudoGID)
	}
	ctx = service.ContextWith(ctx, deprecated.NewStderrManager(log.StdLogger()))
	return box.Context(ctx, include.InboundRegistry(), include.OutboundRegistry(), coreinclude.EndpointRegistry(), include.DNSTransportRegistry(), include.ServiceRegistry())
}
