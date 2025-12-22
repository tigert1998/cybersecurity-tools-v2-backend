package main

import (
	"encoding/json"
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
	Port              = 5000
)

type VersionInfo struct {
	Version string `json:"version"`
	Path    string `json:"path"`
}

type Config struct {
	Versions []VersionInfo `json:"versions"`
}

var logger *slog.Logger
var configCache atomic.Value

func init() {
	file, err := os.OpenFile(LogFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic("无法创建日志文件：" + err.Error())
	}

	logger = slog.New(slog.NewJSONHandler(file, nil))
}

func loadConfig() (Config, error) {
	data, err := os.ReadFile(VersionConfigPath)
	if err != nil {
		return Config{}, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, err
	}

	sort.Slice(config.Versions, func(i, j int) bool {
		return semver.Compare("v"+config.Versions[i].Version, "v"+config.Versions[j].Version) > 0
	})
	return config, nil
}

func startConfigReloader() {
	config, err := loadConfig()
	if err != nil {
		logger.Error("首次加载配置失败", "error", err)
	} else {
		configCache.Store(config)
	}

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for range ticker.C {
			config, err := loadConfig()
			if err != nil {
				logger.Error("刷新配置失败", "error", err)
				continue
			}
			configCache.Store(config)
		}
	}()
}

func LimitMiddleware(limit int) gin.HandlerFunc {
	semaphore := make(chan struct{}, limit) // 使用 struct{} 节省内存

	return func(c *gin.Context) {
		select {
		case semaphore <- struct{}{}:
			defer func() { <-semaphore }() // 确保即使 panic 也能释放
			c.Next()
		default:
			c.AbortWithStatus(http.StatusTooManyRequests) // 明确返回 429
			return
		}
	}
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	startConfigReloader()

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(LimitMiddleware(2000))

	r.GET("/latest_version", func(c *gin.Context) {
		value := configCache.Load()
		if value == nil {
			c.String(http.StatusServiceUnavailable, "Config not loaded")
			return
		}

		config := value.(Config)
		if len(config.Versions) == 0 {
			c.String(http.StatusNotFound, "No versions found")
			return
		}

		c.String(http.StatusOK, config.Versions[0].Version)
	})

	r.GET("/download/:version", func(c *gin.Context) {
		version := c.Param("version")

		value := configCache.Load()
		if value == nil {
			c.String(http.StatusServiceUnavailable, "Config not loaded")
			return
		}
		config := value.(Config)

		for _, obj := range config.Versions {
			if obj.Version == version {
				filePath := filepath.Join(PackageStoreDir, obj.Path)
				c.FileAttachment(filePath, obj.Path)
				return
			}
		}

		c.String(http.StatusNotFound, "Version not found")
	})

	fmt.Printf("服务器运行于：0.0.0.0:%v\n", Port)
	if err := r.Run(fmt.Sprintf("0.0.0.0:%v", Port)); err != nil {
		logger.Error("服务器启动失败", "error", err)
	}
}
