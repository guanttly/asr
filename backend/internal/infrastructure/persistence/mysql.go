package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	pkgconfig "github.com/lgt/asr/pkg/config"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	schemaMigrationLockName       = "asr:schema_migration"
	schemaMigrationLockTimeoutSec = 60
)

// NewMySQL initializes a GORM DB connection.
func NewMySQL(cfg pkgconfig.DatabaseConfig, logger *zap.Logger) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(cfg.DSN()), &gorm.Config{
		Logger: gormlogger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), gormlogger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to mysql: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	logger.Info("mysql connected", zap.String("host", cfg.Host), zap.String("db", cfg.DBName))

	return db, nil
}

// AutoMigrate creates the initial schema for the current bounded contexts.
func AutoMigrate(db *gorm.DB) error {
	conn, err := acquireMigrationLock(db, schemaMigrationLockName, schemaMigrationLockTimeoutSec)
	if err != nil {
		return err
	}
	defer releaseMigrationLock(conn, schemaMigrationLockName)

	return db.AutoMigrate(
		&TaskModel{},
		&AdminOperationStateModel{},
		&MeetingModel{},
		&TranscriptModel{},
		&SummaryModel{},
		&DictModel{},
		&EntryModel{},
		&RuleModel{},
		&FillerDictModel{},
		&FillerEntryModel{},
		&SensitiveDictModel{},
		&SensitiveEntryModel{},
		&VoiceCommandDictModel{},
		&VoiceCommandEntryModel{},
		&UserModel{},
		&DeviceIdentityModel{},
		&UserWorkflowBindingsModel{},
		&WorkflowModel{},
		&WorkflowNodeModel{},
		&WorkflowNodeDefaultModel{},
		&WorkflowExecutionModel{},
		&WorkflowNodeResultModel{},
		&AppSettingModel{},
		&OpenAppModel{},
		&OpenAppCapabilityModel{},
		&OpenSkillModel{},
		&OpenCallLogModel{},
		&SkillInvocationModel{},
	)
}

func acquireMigrationLock(db *gorm.DB, lockName string, timeoutSec int) (*sql.Conn, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, err
	}

	var locked int
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", lockName, timeoutSec).Scan(&locked); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	if locked != 1 {
		_ = conn.Close()
		return nil, fmt.Errorf("migration lock %q not acquired within %d seconds", lockName, timeoutSec)
	}

	return conn, nil
}

func releaseMigrationLock(conn *sql.Conn, lockName string) {
	if conn == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var released sql.NullInt64
	_ = conn.QueryRowContext(ctx, "SELECT RELEASE_LOCK(?)", lockName).Scan(&released)
	_ = conn.Close()
}
