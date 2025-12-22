package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"

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

func init() {
	file, err := os.OpenFile(LogFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic("无法创建日志文件：" + err.Error())
	}

	logger = slog.New(slog.NewJSONHandler(file, nil))
}

func main() {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/latest_version", func(c *gin.Context) {
		data, err := os.ReadFile(VersionConfigPath)
		if err != nil {
			logger.Error("读取配置文件失败", "error", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		var config Config
		if err := json.Unmarshal(data, &config); err != nil {
			logger.Error("解析JSON失败", "error", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		if len(config.Versions) == 0 {
			c.String(http.StatusNotFound, "No versions found")
			return
		}

		sort.Slice(config.Versions, func(i, j int) bool {
			return semver.Compare("v"+config.Versions[i].Version, "v"+config.Versions[j].Version) > 0
		})

		c.String(http.StatusOK, config.Versions[0].Version)
	})

	r.GET("/download/:version", func(c *gin.Context) {
		version := c.Param("version")

		data, err := os.ReadFile(VersionConfigPath)
		if err != nil {
			logger.Error("读取配置文件失败", "error", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		var config Config
		json.Unmarshal(data, &config)

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
