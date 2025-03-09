package main

import (
	"cmp"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

func logError(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

const GO_MG_ENV_VAR string = "GO_MG_MIGRATIONS_DIR"
const DATABASE_CONNECTION string = "DATABASE_CONNECTION"

type App struct {
	migrationsDir            string
	databaseConnectionString string
}

func main() {
	createMigrationCmd := flag.NewFlagSet("create", flag.ExitOnError)
	migrateMigrationCmd := flag.NewFlagSet("migrate", flag.ExitOnError)
	seedCmd := flag.NewFlagSet("seed", flag.ExitOnError)
	seedSqlFile := seedCmd.String("path", "", "The path to the file to be seeded")

	app := &App{}

	switch os.Args[1] {
	case "create":
		createMigrationCmd.Parse(os.Args[2:])
		setupApp(app)
		createMigrationCommand(app)
	case "migrate":
		migrateMigrationCmd.Parse(os.Args[2:])
		setupApp(app)
		runMigrationCommand(app)
	case "seed":
		seedCmd.Parse(os.Args[2:])
		setupApp(app)
		seedDatabase(app, *seedSqlFile)
	case "help":
		createMigrationCmd.Usage()
		// migrateMigrationCmd.Usage()
		seedCmd.Usage()
	}
}

func seedDatabase(app *App, pathOfFileToSeed string) {
	file, err := os.ReadFile(pathOfFileToSeed)
	logError(err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db := openDB(app)

	tx, err := db.BeginTx(ctx, nil)
	logError(err)

	_, err = tx.Exec(string(file))

	if err != nil {
		tx.Rollback()
		logError(err)
	}

	tx.Commit()
}

func runMigrationCommand(app *App) {
	db := openDB(app)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	migrationTableExists := migrationsTableExists(db, ctx)
	var lastMigration string

	if !migrationTableExists {
		_, err := db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS migration (name VARCHAR(255) PRIMARY KEY);")
		logError(err)
	}

	err := db.QueryRowContext(ctx, "select name from migration;").Scan(&lastMigration)
	if err != sql.ErrNoRows {
		logError(err)
	}

	migrationVersions := getSortedMigrations(app)
	startingMigrationIndex := 0

	for migrationIndex, migrationEntry := range migrationVersions {
		if migrationEntry.Name() == lastMigration {
			startingMigrationIndex = migrationIndex + 1
		}
	}

	if startingMigrationIndex >= len(migrationVersions) {
		fmt.Println("Migrations up to date")
		return
	}

	migrateSqlFiles(app, db, ctx, migrationVersions[startingMigrationIndex:])
}

func migrateSqlFiles(app *App, db *sql.DB, ctx context.Context, migrationFiles []os.DirEntry) {
	tx, err := db.BeginTx(ctx, nil)
	logError(err)
	var lastProcessedMigration string

	for _, migrationEntry := range migrationFiles {
		upFile, err := os.ReadFile(filepath.Join(app.migrationsDir, migrationEntry.Name(), "up.sql"))
		logError(err)

		_, err = tx.Exec(string(upFile))

		if err != nil {
			tx.Rollback()
			logError(err)
		}

		lastProcessedMigration = migrationEntry.Name()
	}

	result, err := tx.Exec("update migration set name = $1;", lastProcessedMigration)
	if err != nil {
		tx.Rollback()
		logError(err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		tx.Rollback()
		logError(err)
	}

	if rowsAffected == 0 {
		_, err = tx.Exec("insert into migration (name) values ($1);", lastProcessedMigration)
		if err != nil {
			tx.Rollback()
			logError(err)
		}
	}

	tx.Commit()
}

func getSortedMigrations(app *App) []os.DirEntry {
	migrationVersions, err := os.ReadDir(filepath.Join(app.migrationsDir))
	logError(err)

	slices.SortFunc(migrationVersions, func(a, b os.DirEntry) int {
		splitMigrationA := strings.Split(a.Name(), "-")
		timestampAString := splitMigrationA[len(splitMigrationA)-1]
		timestampA, err := strconv.Atoi(timestampAString)
		logError(err)

		splitMigrationB := strings.Split(b.Name(), "-")
		timestampBString := splitMigrationB[len(splitMigrationB)-1]
		timestampB, err := strconv.Atoi(timestampBString)
		logError(err)

		return cmp.Compare(timestampA, timestampB)
	})

	return migrationVersions
}

func createMigrationCommand(app *App) {
	// Get new migration name from user
	var name string
	fmt.Print("Enter migration name (spaces in the name are not allowed): ")
	fmt.Scan(&name)

	// Create new migration directory
	newMigrationDirName := name + "-" + fmt.Sprint(time.Now().Unix())
	newMigrationDirPath := filepath.Join(app.migrationsDir, newMigrationDirName)
	err := os.MkdirAll(newMigrationDirPath, 0770)
	logError(err)

	// Create new migration up and down files
	upMigrationFilePath := filepath.Join(newMigrationDirPath, "up.sql")
	downMigrationFilePath := filepath.Join(newMigrationDirPath, "down.sql")
	err = os.WriteFile(upMigrationFilePath, []byte(""), 0770)
	logError(err)
	err = os.WriteFile(downMigrationFilePath, []byte(""), 0770)
	logError(err)
}

func openDB(app *App) *sql.DB {
	pool, err := sql.Open("postgres", app.databaseConnectionString)
	logError(err)

	if err = pool.Ping(); err != nil {
		log.Fatal(err)
	}

	return pool
}

func migrationsTableExists(db *sql.DB, ctx context.Context) bool {
	var exists bool
	err := db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM pg_catalog.pg_tables 
		WHERE schemaname = 'public' 
		AND tablename = 'migration'
	);`).Scan(&exists)
	logError(err)

	return exists
}

func setupApp(app *App) {
	migrationsDir := os.Getenv(GO_MG_ENV_VAR)

	if migrationsDir == "" {
		fmt.Printf("The %v environment variable has to be set\n", GO_MG_ENV_VAR)
		os.Exit(0)
	}

	app.migrationsDir = migrationsDir

	databaseConnectionString := os.Getenv(DATABASE_CONNECTION)

	if databaseConnectionString == "" {
		fmt.Printf("The %v environment variable has to be set\n", DATABASE_CONNECTION)
		os.Exit(0)
	}

	app.databaseConnectionString = databaseConnectionString
}

