package zeus

import (
	"fmt"
	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/hydra"
	"github.com/hydraide/hydraide/app/core/hydra/lock"
	"github.com/hydraide/hydraide/app/core/safeops"
	"github.com/hydraide/hydraide/app/core/settings"
	"log/slog"
	"os"
)

type Zeus interface {
	// InitDataFolder initializes the data folder for the hydra
	InitDataFolder() error
	// GetHydra the hydra interface from the Zeus
	GetHydra() hydra.Hydra
	// GetSafeops the safeops interface from the Zeus
	GetSafeops() safeops.Safeops
	// GetSettings the settings interface from the Zeus
	GetSettings() settings.Settings
	// StartHydra the Hydra
	StartHydra()
	// StopHydra graceful stops the hydra
	StopHydra()
}

type zeus struct {
	settingsInterface   settings.Settings
	safeopsInterface    safeops.Safeops
	hydraInterface      hydra.Hydra
	filesystemInterface filesystem.Filesystem
}

func New(settingsInterface settings.Settings, filesystemInterface filesystem.Filesystem) Zeus {
	z := &zeus{
		settingsInterface:   settingsInterface,
		filesystemInterface: filesystemInterface,
	}
	return z
}

func (z *zeus) InitDataFolder() error {

	// Lekérjük az abszolút elérési utat az adatfolderhez
	absPath := z.settingsInterface.GetHydraAbsDataFolderPath()
	// Ellenőrizzük, hogy az útvonal nem üres
	if absPath == "" {
		return fmt.Errorf("the data folder path is empty")
	}

	// Ellenőrizzük, hogy a mappa már létezik-e
	if _, err := os.Stat(absPath); err == nil {
		// A mappa már létezik, nem kell semmit tenni
		return nil
	} else if !os.IsNotExist(err) {
		// Ha valami más hiba történt az ellenőrzés során, visszaadjuk az errort
		return fmt.Errorf("error checking data folder: %s", err)
	}

	// Megpróbáljuk létrehozni az adatfoldert (és a hiányzó szülőmappákat, ha szükséges)
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return fmt.Errorf("error creating data folder: %s", err)
	}

	return nil

}

func (z *zeus) GetSettings() settings.Settings {
	return z.settingsInterface
}

func (z *zeus) GetSafeops() safeops.Safeops {
	return z.safeopsInterface
}

func (z *zeus) GetHydra() hydra.Hydra {
	return z.hydraInterface
}

func (z *zeus) StartHydra() {

	slog.Info("HydrAIDE DataEngine is starting...")

	z.safeopsInterface = safeops.New()

	go func() {
		for {
			select {
			case <-z.safeopsInterface.MonitorPanic():
				slog.Error("Zeus is stopping the HydrAIDE because there was a panic in the system")
				z.StopHydra()
				return
			}
		}
	}()

	// hashRing interface init
	// create new hydra interface
	z.hydraInterface = hydra.New(z.settingsInterface, z.safeopsInterface, lock.New(), z.filesystemInterface)

}

func (z *zeus) StopHydra() {

	// Stops the hydra and all the swamps
	// this is a blocker function until all well are stopped gracefully
	z.hydraInterface.GracefulStop()

	// WaitForUnlock waits until the system releases the transaction. The function returns when there are no more active
	// transaction requests in the system.
	z.safeopsInterface.WaitForUnlock()

}
