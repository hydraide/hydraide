package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/certificate"
	"github.com/hydraide/hydraide/app/hydraidectl/cmd/utils/filesystem"
	"github.com/spf13/cobra"
)

// certCmd defines a CLI subcommand that ONLY generates TLS certificates
// without modifying or reinitializing any running instances/containers.
// Useful when you need to (re)issue certs separately from the full init flow.
var certCmd = &cobra.Command{
	Use:   "cert",
	Short: "Generate certificates only, without modifying instances",
	Long: `This command generates all required certificate files for a Docker container,
or replaces certificates for an existing instance without re-initializing it.
Do NOT use this during a fresh installation — the 'init' command already includes
certificate generation steps.`,

	Run: func(cmd *cobra.Command, args []string) {
		// Reader for interactive stdin prompts
		reader := bufio.NewReader(os.Stdin)

		// Filesystem helper (abstracted for testability and consistency)
		fs := filesystem.New()

		// Ask for the destination folder where the generated cert files will be placed
		fmt.Println("\n📁 Certificate folder")
		fmt.Println("Please provide the directory where you want to place the certificate files.")
		fmt.Println("Make sure the directory exists and is writable.")
		fmt.Print("Your certificate folder path: ")
		certFolder, _ := reader.ReadString('\n')
		certFolder = strings.TrimSpace(certFolder)

		// Launch interactive certificate prompts (CN, SANs, etc.)
		certPrompts := certificate.NewPrompts()
		certPrompts.Start(reader)

		// Print a concise summary before proceeding
		fmt.Println("\n🔧 Configuration Summary:")
		fmt.Println("  • CN:         ", certPrompts.GetCN())
		fmt.Println("  • DNS SANs:   ", strings.Join(certPrompts.GetDNS(), ", "))
		fmt.Println("  • IP SANs:    ", strings.Join(certPrompts.GetIP(), ", "))

		// Final confirmation (default: no)
		fmt.Print("\n✅ Proceed with certificate generation? (y/n) [default: n]: ")
		confirm, _ := reader.ReadString('\n')
		confirm = strings.ToLower(strings.TrimSpace(confirm))
		if confirm != "y" && confirm != "yes" {
			fmt.Println("🚫 Generation cancelled.")
			return
		}

		fmt.Println("\n✅ Starting certificate generation...")

		ctx := context.Background()

		// Validate that target directory exists
		exists, err := fs.CheckIfDirExists(ctx, certFolder)
		if err != nil {
			fmt.Printf("❌ Error checking directory %s: %v\n", certFolder, err)
			return
		}
		if !exists {
			fmt.Printf("❌ Directory does not exist: %s\n", certFolder)
			return
		}

		// Generate the certificate/key material to temporary/local files
		certPrompts.GenerateCert()

		// Move generated files to the target directory
		fmt.Println("\n📂 Moving TLS certificates to the certificate directory...")

		for _, file := range certPrompts.GetCertificateFiles() {
			destPath := filepath.Join(certFolder, filepath.Base(file))
			fmt.Printf("  • Moving %s to %s\n", file, destPath)
			if err := fs.MoveFile(ctx, file, destPath); err != nil {
				fmt.Println("❌ Error moving certificate file:", err)
				return
			}
			fmt.Printf("✅ Moved %s to %s\n", file, destPath)
		}

		fmt.Println("✅ TLS certificates moved successfully.")
	},
}

func init() {
	// Register the 'cert' subcommand under the root command
	rootCmd.AddCommand(certCmd)
}
