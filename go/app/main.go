package main

import (
	"crypto/sha256"
	"database/sql"
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
	DBPath = "./db/mercari.sqlite3"
)

type Response struct {
	Message string `json:"message"`
}

type ReturnItem struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Image    string `json:"image_name"`
}

type Item struct {
	Name       string `json:"name"`
	CategoryId int    `json:"category_id"`
	Image      string `json:"image_name"`
}

type Items struct {
	Items []*ReturnItem `json:"items"`
}

func parseError(c echo.Context, message string, err error) {
	res := Response{Message: message}
	c.JSON(http.StatusInternalServerError, res)
	c.Logger().Error(err)
}

func root(c echo.Context) error {
	res := Response{Message: "Hello, world!"}
	return c.JSON(http.StatusOK, res)
}

func getHashedImage(c echo.Context) (string, error) {
	imageFile, err := c.FormFile("image_name")
	if err != nil {
		parseError(c, "Failed to get image file", err)
		return "", err
	}

	src, err := imageFile.Open()
	if err != nil {
		parseError(c, "Failed to open image file", err)
		return "", err
	}
	defer src.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, src); err != nil {
		parseError(c, "Failed to hash image file", err)
		return "", err
	}

	src.Seek(0, 0)
	hashedImage := fmt.Sprintf("%x.jpg", hash.Sum(nil))

	dst, err := os.Create(path.Join(ImgDir, hashedImage))
	if err != nil {
		parseError(c, "Failed to create image file", err)
		return "", err
	}

	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		parseError(c, "Failed to copy image file", err)
		return "", err
	}

	return hashedImage, nil
}

func getItems(c echo.Context) error {
	rows, err := db.Query("SELECT items.name, categories.name, items.image_name FROM items JOIN categories ON items.category_id = categories.id;")
	if err != nil {
		parseError(c, "Failed to get items from DB", err)
		return err
	}
	defer rows.Close()

	items := Items{Items: []*ReturnItem{}}
	for rows.Next() {
		var item ReturnItem
		err = rows.Scan(&item.Name, &item.Category, &item.Image)
		if err != nil {
			parseError(c, "Failed to scan rows", err)
			return err
		}
		items.Items = append(items.Items, &item)
	}
	return c.JSON(http.StatusOK, items)
}

func addItem(c echo.Context) error {
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS items (id INTEGER PRIMARY KEY, name TEXT, category_id INTEGER, image_name TEXT)")
	if err != nil {
		parseError(c, "Failed to create table", err)
		return err
	}

	name := c.FormValue("name")
	categoryId := c.FormValue("category_id")
	categoryIdInt, err := strconv.Atoi(categoryId)
	if err != nil {
		parseError(c, "Failed to convert category_id to int", err)
		return err
	}

	hashedImage, err := getHashedImage(c)
	if err != nil {
		parseError(c, "Failed to get hashed image", err)
		return err
	}

	_, err = db.Exec("INSERT INTO items (name, category_id, image_name) VALUES (?, ?, ?)", name, categoryIdInt, hashedImage)
	if err != nil {
		parseError(c, "Failed to insert item into database", err)
		return err
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "item added"})
}

func getItemById(c echo.Context) error {
	id := c.Param("id")
	var item Item
	query := "SELECT name, category_id, image_name FROM items WHERE id = ?"
	err = db.QueryRow(query, id).Scan(&item.Name, &item.CategoryId, &item.Image)
	if err != nil {
		if err == sql.ErrNoRows {
			parseError(c, "Item not found", err)
			return err
		}
		parseError(c, "Failed to query item by ID", err)
		return err
	}

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

	query := `SELECT name, category_id FROM items WHERE name LIKE ?`
	rows, err := db.Query(query, "%"+keyword+"%")
	if err != nil {
		parseError(c, "Failed to query items", err)
		return err
	}
	defer rows.Close()

	var items []map[string]string
	for rows.Next() {
		var name string
		var category_id int
		if err := rows.Scan(&name, &category_id); err != nil {
			parseError(c, "Failed to scan rows", err)
			return err
		}
		items = append(items, map[string]string{"name": name, "category_id": strconv.Itoa(category_id)})
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

	db, err := sql.Open("sqlite3", DBPath)
	if err != nil {
		parseError(c, "Failed to open database", err)
	}
	defer func(db *sql.DB) {
		err := db.Close()
		if err != nil {
			parseError(c, "Failed to close database", err)
		}
	}(db)

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
