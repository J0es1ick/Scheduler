package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/J0es1ick/Scheduler/internal/config"
	"github.com/J0es1ick/Scheduler/internal/database"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	checkDatabase := flag.Bool("database", false, "Проверить соединения всех runtime-ролей после миграций")
	flag.Parse()
	if err := config.ProductionPreflight(os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *checkDatabase {
		cfg, err := config.InitWorkerConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Не удалось загрузить настройки подключения.")
			os.Exit(1)
		}
		for _, role := range databaseRoles() {
			cfg.Database.User, cfg.Database.Password = os.Getenv("DATABASE_"+role+"_USER"), os.Getenv("DATABASE_"+role+"_PASSWORD")
			db, err := database.NewDatabase(cfg)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Ошибка подключения роли", role)
				os.Exit(1)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			var currentUser string
			var ssl, superuser bool
			err = db.DB.QueryRowContext(ctx, `SELECT current_user, s.ssl, r.rolsuper FROM pg_stat_ssl s JOIN pg_roles r ON r.rolname=current_user WHERE s.pid=pg_backend_pid()`).Scan(&currentUser, &ssl, &superuser)
			if err == nil && role == "ADMIN" {
				err = database.CheckSubscriptionIntegrity(ctx, db.DB.DB)
			}
			cancel()
			db.Close()
			if err != nil || !ssl || superuser || currentUser != cfg.Database.User {
				fmt.Fprintln(os.Stderr, "TLS, права или целостность данных не прошли проверку для", role)
				os.Exit(1)
			}
		}
	}
	fmt.Println("Production preflight пройден. Доступность внешнего HTTPS и восстановление offsite-копии проверяются отдельно.")
}

func databaseRoles() []string {
	return []string{"MIGRATOR", "BOT", "PARSER", "PRIVACY", "ADMIN", "SITE", "BACKUP", "RESTORE"}
}
