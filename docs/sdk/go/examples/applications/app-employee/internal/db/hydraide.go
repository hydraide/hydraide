package db

import (
	"log"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

var (
	// hydraRepo is a global repository instance used for all Hydraide DB operations.
	hydraRepo repo.Repo
)

// Init initializes the Hydraide database connection and sets up the repository.
//
// This function connects to the Hydraide server using the provided host and certificate path.
// It must be called before any database operations are performed.
//
// Example:
//
//	db.Init()
//	repo := db.GetRepo()
func Init() {
	// Host and certificate path for connecting to Hydraide DB.
	host := "localhost:4900"
	// Must be chnaged according to your path
	certPath := `C:\mnt\hydraide\certificate\server.crt`

	// In a real application, these should come from environment variables or config files.
	if host == "" || certPath == "" {
		log.Fatal("host and certPath environment variables must be set.")
	}

	// Define the Hydraide server(s) to connect to.
	servers := []*client.Server{
		{
			Host:         host,
			FromIsland:   1,
			ToIsland:     1000,
			CertFilePath: certPath,
		},
	}

	// Create a new Hydraide repository.
	// 1000 = max islands, 10485760 = 10MB max message size, true = enable logging.
	hydraRepo = repo.New(servers, 1000, 10485760, true)
	log.Println("Successfully connected to Hydraide database.")
}

// GetRepo returns the global Hydraide repository instance.
//
// Use this function to access the repository for all database operations.
//
// Example:
//
//	repo := db.GetRepo()
func GetRepo() repo.Repo {
	return hydraRepo
}

// GetDB returns the Hydraidego instance from the repository.
//
// This is a lower-level API for advanced Hydraide operations.
//
// Example:
//
//	db := db.GetDB()
func GetDB() hydraidego.Hydraidego {
	return hydraRepo.GetHydraidego()
}
