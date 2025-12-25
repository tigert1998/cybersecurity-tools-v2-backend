package main

import (
	"database/sql"
	"fmt"
	"math/rand"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB
var wg sync.WaitGroup

const benchDBPath = "benchmark.db"

func init() {
	var err error
	db, err = sql.Open("sqlite", benchDBPath)
	if err != nil {
		panic("无法打开数据库：" + err.Error())
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL;PRAGMA synchronous=NORMAL;"); err != nil {
		panic("无法开启 WAL 模式：" + err.Error())
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
}

func run(numSqls int, goid int, fromIP int, toIP int) {
	source := rand.NewSource(int64(goid))
	r := rand.New(source)

	for range numSqls {
		insertSQL := "INSERT OR REPLACE INTO users (ip, version, timestamp) VALUES (?, ?, ?);"
		_, err := db.Exec(insertSQL, fmt.Sprintf("%v", r.Intn(toIP-fromIP)+fromIP), "1.0.0", time.Now().Unix())
		if err != nil {
			fmt.Printf("[E] 插入数据失败：%v\n", err)
		}
	}

	wg.Done()
}

func main() {
	numGos := 16
	ipsPerGo := int(6000 / numGos)
	numSqls := 2048

	start := time.Now()
	wg.Add(numGos)
	for i := range numGos {
		go run(numSqls, i, ipsPerGo*i, ipsPerGo*(i+1))
	}
	wg.Wait()
	duration := time.Since(start)
	fmt.Printf("[I] Time: %vs\n", duration.Seconds())
	fmt.Printf("[I] Transactions: %v\n", numSqls*numGos)
	fmt.Printf("[I] TPS: %v\n", float64(numSqls*numGos)/duration.Seconds())
}
