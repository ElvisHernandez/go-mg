package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

func logError(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

func main() {
	createMigrationCmd := flag.NewFlagSet("create", flag.ExitOnError)
	migrationDirPath := createMigrationCmd.String("dir", "./migrations", "The path to the migration directory")
	createMigrationCmd.Parse(os.Args[2:])
	// createMigrationCmd.Usage()

	switch os.Args[1] {
	case "create":
		createMigrationCommand(*migrationDirPath)
	}
}

func createMigrationCommand(migrationsDirPath string) error {
	// Get new migration name from user
	var name string
	fmt.Print("Enter migration name (spaces in the name are not allowed): ")
	fmt.Scan(&name)

	// Create new migration directory
	newMigrationDirName := name + "-" + fmt.Sprint(time.Now().Unix())
	newMigrationDirPath := filepath.Join(migrationsDirPath, newMigrationDirName)
	err := os.MkdirAll(newMigrationDirPath, 0770)
	logError(err)

	// Create new migration up and down files
	upMigrationFilePath := filepath.Join(newMigrationDirPath, "up.sql")
	downMigrationFilePath := filepath.Join(newMigrationDirPath, "down.sql")
	err = os.WriteFile(upMigrationFilePath, []byte(""), 0770)
	logError(err)
	err = os.WriteFile(downMigrationFilePath, []byte(""), 0770)
	logError(err)

	return nil
}
