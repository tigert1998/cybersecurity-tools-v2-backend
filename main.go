package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/mod/semver"
)

const (
	PackageStoreDir   = "./packages"
	VersionConfigPath = "./packages/version.json"
	LogFilePath       = "./log.txt"
	Port              = 38686
)

type VersionInfo struct {
	Version string `json:"version"`
	Path    string `json:"path"`
}

type Config struct {
	Versions []VersionInfo `json:"versions"`
}

type LatestPackage struct {
	version  string
	filepath string
}

var logger *slog.Logger
var latestPackageCache atomic.Value

func init() {
	file, err := os.OpenFile(LogFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic("无法创建日志文件：" + err.Error())
	}

	logger = slog.New(slog.NewJSONHandler(file, nil))
}

func loadLatestPackage() (LatestPackage, error) {
	data, err := os.ReadFile(VersionConfigPath)
	if err != nil {
		return LatestPackage{}, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return LatestPackage{}, err
	}

	if len(config.Versions) == 0 {
		return LatestPackage{}, errors.New("No versions found")
	}

	sort.Slice(config.Versions, func(i, j int) bool {
		return semver.Compare("v"+config.Versions[i].Version, "v"+config.Versions[j].Version) > 0
	})

	path := filepath.Join(PackageStoreDir, config.Versions[0].Path)
	return LatestPackage{version: config.Versions[0].Version, filepath: path}, nil
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

	r.GET("/latest_version", func(c *gin.Context) {
		value := latestPackageCache.Load()
		if value == nil {
			c.String(http.StatusServiceUnavailable, "Package not loaded")
			return
		}

		latestPackage := value.(LatestPackage)
		c.String(http.StatusOK, latestPackage.version)
	})

	r.GET("/download/:version", func(c *gin.Context) {
		version := c.Param("version")

		value := latestPackageCache.Load()
		if value == nil {
			c.String(http.StatusServiceUnavailable, "Package not loaded")
			return
		}
		latestPackage := value.(LatestPackage)
		if latestPackage.version == version {
			c.FileAttachment(latestPackage.filepath, filepath.Base(latestPackage.filepath))
		} else {
			c.String(http.StatusNotFound, "Version not found")
		}
	})

	fmt.Printf("服务器运行于：0.0.0.0:%v\n", Port)
	if err := r.Run(fmt.Sprintf("0.0.0.0:%v", Port)); err != nil {
		logger.Error("服务器启动失败", "error", err)
	}
}
