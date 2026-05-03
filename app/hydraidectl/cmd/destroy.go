package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	buildmeta "github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/buildmetadata"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/instancerunner"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/servicehelper"
	"github.com/spf13/cobra"
)

var (
	destroyInstance        string
	confirmDestroyInstance string
	purgeData              bool
)

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Stops, disables, and removes a HydrAIDE instance.",
	Long: `Stops and removes a HydrAIDE instance.

⚠️ This command will always perform a graceful shutdown and remove the system service definition.
⚠️ Use the --purge flag to also permanently delete the entire base directory, including all data, certificates, and the server binary. 
⚠️ This action is irreversible and requires manual confirmation.
`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()

		printJson, _ := cmd.Flags().GetBool("json")
		forceDestroy, _ := cmd.Flags().GetBool("force")

		jsonResponse := NewJsonDestroyInfo(destroyInstance)

		if !forceDestroy && confirmDestroyInstance != "" && confirmDestroyInstance != destroyInstance {
			if printJson {
				printDestroyJson(UpdateJsonDestroyInfo(jsonResponse, WithErrorMessage("Instance name confirmation failed")))
			} else {
				fmt.Println("❌ Instance name confirmation failed")
			}
			os.Exit(1)
		}

		fs := filesystem.New()
		bm, err := buildmeta.New(fs)
		if err != nil {
			if printJson {
				printDestroyJson(UpdateJsonDestroyInfo(jsonResponse, WithErrorMessage(err.Error())))
			} else {
				fmt.Println("❌ Failed to initialize metadata store:", err)
			}
			os.Exit(1)
		}

		instanceController := instancerunner.NewInstanceController()
		serviceManager := servicehelper.New()

		if !printJson {
			fmt.Printf("🔥 Initializing destruction for instance: \"%s\"\n", destroyInstance)
			fmt.Println("🔄 Checking instance status...")
		}
		err = instanceController.StopInstance(ctx, destroyInstance)
		if err != nil && err != instancerunner.ErrServiceNotRunning && err != instancerunner.ErrServiceNotFound {
			if printJson {
				printDestroyJson(UpdateJsonDestroyInfo(jsonResponse, WithErrorMessage(err.Error()), WithWarning("Could not stop instance")))
			} else {
				fmt.Printf("❌ Could not stop instance '%s': %v\n", destroyInstance, err)
				fmt.Println("🚫 Aborting destroy operation. Please stop the service manually before proceeding.")
			}
			os.Exit(1)
		}
		if !printJson {
			fmt.Println("✅ Instance is stopped or was not running.")
			fmt.Println("🗑️  Removing service definition...")
		}

		if err := serviceManager.RemoveService(destroyInstance); err != nil {
			if !printJson {
				fmt.Printf("⚠️  Could not remove service definition: %v\n", err)
				fmt.Println("   You may need to remove it manually. Continuing...")
			} else {
				UpdateJsonDestroyInfo(jsonResponse, WithServiceStopped())
			}
		} else {
			if !printJson {
				fmt.Println("✅ Service definition removed.")
			} else {
				UpdateJsonDestroyInfo(jsonResponse, WithServiceRemoved(), WithServiceStopped())
			}
		}

		// FIXED: Added delay to prevent console output overlap.
		time.Sleep(500 * time.Millisecond)

		instanceData, metaErr := bm.GetInstance(destroyInstance)
		if metaErr != nil {
			if printJson {
				printDestroyJson(UpdateJsonDestroyInfo(jsonResponse, WithStatus("partial"), WithWarning("Could not find metadata")))
			} else {
				fmt.Printf("⚠️  Could not find metadata for instance '%s'. Cannot purge data.\n", destroyInstance)
				fmt.Printf("✅ Destruction of service '%s' complete.\n", destroyInstance)
			}
			os.Exit(0)
		}
		basePath := instanceData.BasePath

		UpdateJsonDestroyInfo(jsonResponse, WithBasePath(basePath))

		if purgeData {
			if !printJson && !forceDestroy && confirmDestroyInstance == "" {
				fmt.Println("\n====================================================================")
				fmt.Println("‼️ DANGER: FULL DATA PURGE INITIATED ‼️")
				fmt.Printf("⚠️ You are about to permanently delete the entire base path for instance '%s'.\n", destroyInstance)
				fmt.Printf("   Directory to be deleted: %s\n", basePath)
				fmt.Println("   This includes all data, certificates, logs, and the binary.")
				fmt.Println("   This operation is IRREVERSIBLE.")
				fmt.Println("====================================================================")
				fmt.Printf("\n👉 To confirm, type the full instance name ('%s'): ", destroyInstance)

				reader := bufio.NewReader(os.Stdin)
				input, _ := reader.ReadString('\n')
				if strings.TrimSpace(input) != destroyInstance {
					fmt.Println("🚫 Input did not match. Data purge aborted.")
					os.Exit(0)
				}

				fmt.Println("✅ Confirmation received. Proceeding with base path deletion.")
			}

			// FIXED: Use the robust fs.RemoveDir which wraps os.RemoveAll.
			if err := fs.RemoveDir(ctx, basePath); err != nil {
				if printJson {
					printDestroyJson(UpdateJsonDestroyInfo(jsonResponse, WithErrorMessage("An error occurred during deletion:"+err.Error())))
				} else {
					fmt.Printf("❌ An error occurred during deletion: %v\n", err)
					fmt.Println("   Please check file permissions and try again.")
				}
				os.Exit(1)
			}
			if !printJson {
				fmt.Printf("✅ Base path '%s' deleted successfully.\n", basePath)
			} else {
				UpdateJsonDestroyInfo(jsonResponse, WithPurged())
			}
		} else if !printJson {
			fmt.Println("\nℹ️  --purge flag not specified. The service was removed, but all data remains.")
			fmt.Printf("   Data directory: %s\n", basePath)
		}

		// CHANGED: Clean up the instance from the metadata file.
		if err := bm.DeleteInstance(destroyInstance); err != nil {
			if !printJson {
				fmt.Printf("⚠️  Could not remove instance metadata: %v\n", err)
			}
		} else {
			if printJson {
				UpdateJsonDestroyInfo(jsonResponse, WithMetaDataRemoved())
			} else {
				fmt.Println("✅ Instance metadata cleaned up.")
			}
		}
		if printJson {
			printDestroyJson(jsonResponse)
		} else {
			fmt.Printf("\n✅ Destruction of instance '%s' complete.\n", destroyInstance)
		}
		os.Exit(0)
	},
}

func init() {
	rootCmd.AddCommand(destroyCmd)
	destroyCmd.Flags().StringVarP(&destroyInstance, "instance", "i", "", "Name of the HydrAIDE instance to destroy (required)")
	destroyCmd.Flags().BoolVar(&purgeData, "purge", false, "Permanently delete the entire base path for the instance")
	destroyCmd.Flags().BoolP("json", "j", false, "Return structured output in JSON format")
	destroyCmd.Flags().StringVarP(&confirmDestroyInstance, "confirm-name", "c", "", "Confirmation of the HydrAIDE instance to destroy")
	destroyCmd.Flags().BoolP("force", "f", false, "Force destroy instance, skipping the instance name confirmation")

	if err := destroyCmd.MarkFlagRequired("instance"); err != nil {
		fmt.Println("❌ Error marking 'instance' flag as required:", err)
		os.Exit(1)
	}
}

type JsonDestroyInfo struct {
	Instance        string `json:"instance"`
	Action          string `json:"action"`
	Stopped         bool   `json:"stopped"`
	ServiceRemoved  bool   `json:"serviceRemoved"`
	BasePath        string `json:"basePath"`
	Purged          bool   `json:"purged"`
	MetadataRemoved bool   `json:"metadataRemoved"`
	Status          string `json:"status"`
	Warnings        string `json:"warnings"`
	ErrorMessage    string `json:"errorMessage"`
	Timestamp       string `json:"timestamp"`
}

type Option func(*JsonDestroyInfo)

func NewJsonDestroyInfo(instance string, opts ...Option) *JsonDestroyInfo {
	jsonDestroyInfo := &JsonDestroyInfo{
		Instance:       instance,
		Action:         "destroy",
		ServiceRemoved: true,
		Timestamp:      time.Now().Format(time.RFC3339),
	}
	for _, opt := range opts {
		opt(jsonDestroyInfo)
	}
	return jsonDestroyInfo
}

func UpdateJsonDestroyInfo(jsonDestroyInfo *JsonDestroyInfo, opts ...Option) *JsonDestroyInfo {
	for _, opt := range opts {
		opt(jsonDestroyInfo)
	}
	return jsonDestroyInfo
}

func WithBasePath(basePath string) Option {
	return func(jsonDestroyInfo *JsonDestroyInfo) { jsonDestroyInfo.BasePath = basePath }
}

func WithErrorMessage(errorMessage string) Option {
	return func(jsonDestroyInfo *JsonDestroyInfo) {
		jsonDestroyInfo.ErrorMessage = errorMessage
		jsonDestroyInfo.Status = "error"
	}
}

func WithStatus(status string) Option {
	return func(jsonDestroyInfo *JsonDestroyInfo) { jsonDestroyInfo.Status = status }
}

func WithWarning(warning string) Option {
	return func(jsonDestroyInfo *JsonDestroyInfo) { jsonDestroyInfo.Warnings = warning }
}

func WithServiceStopped() Option {
	return func(jsonDestroyInfo *JsonDestroyInfo) {
		jsonDestroyInfo.Stopped = true
		jsonDestroyInfo.Status = "error"
	}
}

func WithServiceRemoved() Option {
	return func(jsonDestroyInfo *JsonDestroyInfo) {
		jsonDestroyInfo.ServiceRemoved = true
		jsonDestroyInfo.Status = "partial"
	}
}

func WithPurged() Option {
	return func(jsonDestroyInfo *JsonDestroyInfo) {
		jsonDestroyInfo.Purged = true
		jsonDestroyInfo.Status = "success"
	}
}

func WithMetaDataRemoved() Option {
	return func(jsonDestroyInfo *JsonDestroyInfo) {
		jsonDestroyInfo.MetadataRemoved = true
	}
}

func printDestroyJson(jsonDestroyInfo *JsonDestroyInfo) {
	outputJSON, err := json.MarshalIndent(jsonDestroyInfo, "", "  ")
	if err != nil {
		fmt.Printf("Error generating JSON output: %v", err)
		os.Exit(1)
	}
	fmt.Println(string(outputJSON))
}
