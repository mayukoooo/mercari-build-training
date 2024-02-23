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
	"strconv"
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

func parseError(c echo.Context, message string, error error) {
	res := Response{Message: message}
	c.JSON(http.StatusInternalServerError, res);
	c.Logger().Error(error);
}

func root(c echo.Context) error {
	res := Response{Message: "Hello, world!"}
	return c.JSON(http.StatusOK, res)
}

func getHashedImage(c echo.Context) (string, error) {
	imageFile, error := c.FormFile("image")
	if error != nil {
		parseError(c, "Failed to get image file", error)
		return "", error
	}

	src, err := imageFile.Open()
	if err != nil {
		parseError(c, "Failed to open image file", error)
		return "", err
	}
	defer src.Close()

	hash := sha256.New()
	if _, error := io.Copy(hash, src); err != nil {
		parseError(c, "Failed to hash image file", error)
		return "", error
	}

	src.Seek(0, 0)
	hashedImage := fmt.Sprintf("%x.jpg", hash.Sum(nil))

	dst, err := os.Create(path.Join(ImgDir, hashedImage))
	if err != nil {
		parseError(c, "Failed to create image file", err)
		return "", err
	}

	defer dst.Close()
	if _, error := io.Copy(dst, src); err != nil {
		parseError(c, "Failed to copy image file", error)
		return "", error
	}

	return hashedImage ,nil
}

func getItems(c echo.Context) error {
	db, error := sql.Open("sqlite3", "../db/mercari.sqlite3")
	if error != nil {
		parseError(c, "Failed to open mercari.sqlite3", error)
		return error
	}
	defer db.Close()

	rows, error := db.Query("SELECT items.name, categories.name, items.image_name FROM items JOIN categories ON items.category_id = categories.id;")
	if error != nil {
		parseError(c, "Failed to get items from DB", error)
		return error
	}
	defer rows.Close()

	items := Items{Items: []*Item{}}
	for rows.Next() {
		var item Item
		error = rows.Scan(&item.Name, &item.Category, &item.Image)
		if error != nil {
			parseError(c, "Failed to scan rows", error)
			return error
		}
		items.Items = append(items.Items, &item)
	}
	return c.JSON(http.StatusOK, items)
}

func addItem(c echo.Context) error {
	db, error := sql.Open("sqlite3", "./mercari.sqlite3")
	if error != nil {
		parseError(c, "Failed to open mercari.sqlite3", error)
		return error
	}
	defer db.Close()

	// テーブルの存在を確認するクエリ
	_, error = db.Exec("CREATE TABLE IF NOT EXISTS items (id INTEGER PRIMARY KEY, name TEXT, category TEXT, image_name TEXT)")
	if error != nil {
		parseError(c, "Failed to create table", error)
		return error
	}

	name := c.FormValue("name")
	category := c.FormValue("category")
	hashedImage, error := getHashedImage(c)
	if error != nil {
		parseError(c, "Failed to get hashed image", error)
		return error
	}

	_, error = db.Exec("INSERT INTO items (name, category, image_name) VALUES (?, ?, ?)", name, category, hashedImage)
	if error != nil {
		parseError(c, "Failed to insert item", error)
        return error
	}

	newItem := Item{Name: name, Category: category, Image: hashedImage}
	message := fmt.Sprintf("item received: %s", newItem)
	res := Response{Message: message}

	return c.JSON(http.StatusOK, res)
}

func getItemById(c echo.Context) error {
	data, error := os.ReadFile(itemsJson)
	if error != nil {
		parseError(c, "Failed to read items.json", error)
		return error
	}

	var items Items
	newData := bytes.NewReader(data)
	if error := json.NewDecoder(newData).Decode(&items); error != nil {
		parseError(c,"Failed to newDecoder items.json", error)
		return error
	}

	id := c.Param("id")
	idInt, err := strconv.Atoi(id)
	if err != nil {
		parseError(c, "Invalid ID format", err)
		return err
	}

	if idInt <= 0 || idInt > len(items.Items) {
		res := Response{Message: "Item not found"}
		return c.JSON(http.StatusNotFound, res)
	}

	item := items.Items[idInt - 1]
	return c.JSON(http.StatusOK, item)
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
	e.GET("/search", searchItems)

	// Start server
	e.Logger.Fatal(e.Start(":9000"))
}
