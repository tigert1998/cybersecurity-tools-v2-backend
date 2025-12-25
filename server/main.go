package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

const (
	PackageStoreDir   = "./packages"
	VersionConfigPath = "./packages/version.json"
	LogFilePath       = "./log.txt"
	DBPath            = "./main.db"
	Port              = 38686
)

type VersionInfo struct {
	Version string `json:"version"`
	Path    string `json:"path"`
}

var logger *slog.Logger
var db *sql.DB
var latestPackageCache atomic.Value

func init() {
	var err error
	db, err = sql.Open("sqlite", DBPath)
	if err != nil {
		panic("无法打开数据库：" + err.Error())
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL;PRAGMA synchronous=OFF;"); err != nil {
		panic("无法设置Sqlite：" + err.Error())
	}
	createTableSQL := `
CREATE TABLE IF NOT EXISTS users (
	ip TEXT NOT NULL PRIMARY KEY,
	version TEXT NOT NULL,
	timestamp INTEGER NOT NULL
);`
	_, err = db.Exec(createTableSQL)
	if err != nil {
		panic("创建数据表失败：" + err.Error())
	}

	file, err := os.OpenFile(LogFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic("无法创建日志文件：" + err.Error())
	}

	logger = slog.New(slog.NewJSONHandler(file, nil))
}

func loadLatestPackage() (VersionInfo, error) {
	data, err := os.ReadFile(VersionConfigPath)
	if err != nil {
		return VersionInfo{}, err
	}

	var versionInfo VersionInfo
	if err := json.Unmarshal(data, &versionInfo); err != nil {
		return VersionInfo{}, err
	}

	return versionInfo, nil
}

func startReloadLatestPackage() {
	latestPackage, err := loadLatestPackage()
	if err != nil {
		logger.Error("首次加载配置失败", "error", err)
	} else {
		latestPackageCache.Store(latestPackage)
	}

	go func() {
		for true {
			time.Sleep(time.Second)
			latestPackage, err := loadLatestPackage()
			if err != nil {
				logger.Error("刷新配置失败", "error", err)
				continue
			}
			latestPackageCache.Store(latestPackage)
		}
	}()
}

func LimitMiddleware(limit int) gin.HandlerFunc {
	semaphore := make(chan struct{}, limit)

	return func(c *gin.Context) {
		select {
		case semaphore <- struct{}{}:
			defer func() { <-semaphore }()
			c.Next()
		default:
			c.AbortWithStatus(http.StatusTooManyRequests)
			return
		}
	}
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	startReloadLatestPackage()

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(LimitMiddleware(2000))

	r.GET("/version", func(c *gin.Context) {
		value := latestPackageCache.Load()
		if value == nil {
			c.String(http.StatusServiceUnavailable, "Package not loaded")
			return
		}

		latestPackage := value.(VersionInfo)
		c.String(http.StatusOK, latestPackage.Version)

		clientVersion := strings.TrimSpace(c.Query("q"))
		if clientVersion != "" {
			insertSQL := "INSERT OR REPLACE INTO users (ip, version, timestamp) VALUES (?, ?, ?);"
			_, err := db.Exec(insertSQL, c.ClientIP(), clientVersion, time.Now().Unix())
			if err != nil {
				logger.Error("插入数据失败", "error", err)
			}
		}
	})

	r.GET("/download", func(c *gin.Context) {
		value := latestPackageCache.Load()
		if value == nil {
			c.String(http.StatusServiceUnavailable, "Package not loaded")
			return
		}
		latestPackage := value.(VersionInfo)
		c.FileAttachment(filepath.Join(PackageStoreDir, latestPackage.Path), filepath.Base(latestPackage.Path))
	})

	logger.Info(fmt.Sprintf("服务器运行于：0.0.0.0:%v", Port))
	if err := r.Run(fmt.Sprintf("0.0.0.0:%v", Port)); err != nil {
		logger.Error("服务器启动失败", "error", err)
	}
}
