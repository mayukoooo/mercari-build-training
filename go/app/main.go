package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	_ "github.com/mattn/go-sqlite3"
)

const (
	ImgDir = "images"
	itemsJson = "./items.json"
)

type Response struct {
	Message string `json:"message"`
}

type Item struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Image    string `json:"image_name"`
}

type Items struct {
	Items []*Item `json:"items"`
}

// TODO: エラーが出ても処理が続行されるので修正する
func getErrorStatus(c echo.Context, message string) error {
	c.Logger().Error(message)
	res := Response{Message: message}
	return c.JSON(http.StatusInternalServerError, res)
}

func root(c echo.Context) error {
	res := Response{Message: "Hello, world!"}
	return c.JSON(http.StatusOK, res)
}

func getHashedImage(c echo.Context) string {
	imageFile, error := c.FormFile("image")
	if error != nil {
		getErrorStatus(c, "Failed to get image file")
	}

	src, err := imageFile.Open()
	if err != nil {
		getErrorStatus(c, "Failed to open image file")
	}
	defer src.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, src); err != nil {
		getErrorStatus(c, "Failed to hash image file")
	}

	src.Seek(0, 0)
	hashedImage := fmt.Sprintf("%x.jpg", hash.Sum(nil))

	dst, err := os.Create(path.Join(ImgDir, hashedImage))
	if err != nil {
		getErrorStatus(c, "Failed to create image file")
	}

	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		getErrorStatus(c, "Failed to copy image file")
	}

	return hashedImage
}

func getItems(c echo.Context) error {
	db, err := sql.Open("sqlite3", "../db/mercari.sqlite3")
	if err != nil {
		getErrorStatus(c, "Failed to copy image file")
	}
	defer db.Close()

	rows, err := db.Query("SELECT name, category, image_name FROM items")
	if err != nil {
		getErrorStatus(c, "Failed to get items from DB")
	}
	defer rows.Close()

	items := Items{Items: []*Item{}}
	for rows.Next() {
		var item Item
		err = rows.Scan(&item.Name, &item.Category, &item.Image)
		if err != nil {
			getErrorStatus(c, "Failed to scan rows")
		}
		items.Items = append(items.Items, &item)
	}
	return c.JSON(http.StatusOK, items)
}

func addItem(c echo.Context) error {
	db, err := sql.Open("sqlite3", "./mercari.sqlite3")
	if err != nil {
		getErrorStatus(c, "Failed to open mercari.sqlite3")
	}
	defer db.Close()

	// テーブルの存在を確認するクエリ
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS items (id INTEGER PRIMARY KEY, name TEXT, category TEXT, image_name TEXT)")
	if err != nil {
	    getErrorStatus(c, "Failed to create table")
	}

	name := c.FormValue("name")
	category := c.FormValue("category")
	hashedImage := getHashedImage(c)

	_, err = db.Exec("INSERT INTO items (name, category, image_name) VALUES (?, ?, ?)", name, category, hashedImage)
	if err != nil {
		getErrorStatus(c, "Failed to insert item")
	}

	newItem := Item{Name: name, Category: category, Image: hashedImage}
	message := fmt.Sprintf("item received: %s", newItem)
	res := Response{Message: message}

	return c.JSON(http.StatusOK, res)
}

func getItemById(c echo.Context) error {
	id := c.Param("id")
	data, err := os.ReadFile(itemsJson)
	if err != nil {
		getErrorStatus(c, "Failed to read items.json")
	}

	var items Items
	newData := bytes.NewReader(data)
	if error := json.NewDecoder(newData).Decode(&items); error != nil {
		getErrorStatus(c,"Failed to newDecoder items.json")
	}

	for _, item := range items.Items {
		if item.ID == id {
			return c.JSON(http.StatusOK, item)
		}
	}

	res := Response{Message: "Item not found"}
	return c.JSON(http.StatusNotFound, res)
}

func getImg(c echo.Context) error {
	// Create image path
	imgPath := path.Join(ImgDir, c.Param("imageFilename"))

	if !strings.HasSuffix(imgPath, ".jpg") {
		res := Response{Message: "Image path does not end with .jpg"}
		return c.JSON(http.StatusBadRequest, res)
	}
	if _, err := os.Stat(imgPath); err != nil {
		c.Logger().Debugf("Image not found: %s", imgPath)
		imgPath = path.Join(ImgDir, "default.jpg")
	}
	return c.File(imgPath)
}

func searchItems(c echo.Context) error {
    keyword := c.QueryParam("keyword")
    db, err := sql.Open("sqlite3", "./mercari.sqlite3")
    if err != nil {
        return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to connect to database"})
    }
    defer db.Close()

    query := `SELECT name, category FROM items WHERE name LIKE ?`
    rows, err := db.Query(query, "%"+keyword+"%")
    if err != nil {
        return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to query items"})
    }
    defer rows.Close()

    var items []map[string]string
    for rows.Next() {
        var name, category string
        if err := rows.Scan(&name, &category); err != nil {
            return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to read query results"})
        }
        items = append(items, map[string]string{"name": name, "category": category})
    }

    return c.JSON(http.StatusOK, map[string]interface{}{"items": items})
}

func main() {
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Logger.SetLevel(log.INFO)

	front_url := os.Getenv("FRONT_URL")
	if front_url == "" {
		front_url = "http://localhost:3000"
	}
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{front_url},
		AllowMethods: []string{http.MethodGet, http.MethodPut, http.MethodPost, http.MethodDelete},
	}))

	// Routes
	e.GET("/", root)
	e.POST("/items", addItem)
	e.GET("/items", getItems)
	e.GET("/items/:id", getItemById)
	e.GET("/image/:imageFilename", getImg)
	e.GET("/search", searchItems) // Added searchItems route

	// Start server
	e.Logger.Fatal(e.Start(":9000"))
}
