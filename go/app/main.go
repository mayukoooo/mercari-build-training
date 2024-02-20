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
)

const (
	ImgDir = "images"
	itemsJson = "./items.json"
)

type Response struct {
	Message string `json:"message"`
}

type Item struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Image    string `json:"image_name"`
}

type ItemList struct {
	Items []Item `json:"items"`
}

func getErrorStatus(c echo.Context, message string) error {
	c.Logger().Error(message)
	res := Response{Message: message}
	return c.JSON(http.StatusInternalServerError, res)
}

func root(c echo.Context) error {
	res := Response{Message: "Hello, world!"}
	return c.JSON(http.StatusOK, res)
}

func getItems(c echo.Context) error {
	data, err := os.ReadFile(itemsJson)
	if err != nil {
		return err
	}
	return c.JSONBlob(http.StatusOK, data)
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

func getAllItemsFromDB(db *sql.DB, c echo.Context) echo.HandlerFunc  {
	db, err := sql.Open("sqlite3", "./mercari.sqlite3")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT * FROM items")
	if err != nil {
		getErrorStatus(c, "Failed to get items from DB")
	}
	defer rows.Close()

	var itemList ItemList
	for rows.Next() {
		var item Item
		err = rows.Scan(&item.ID, &item.Name, &item.Category, &item.Image)
		if err != nil {
			getErrorStatus(c, "Failed to scan rows")
		}
		itemList.Items = append(itemList.Items, item)
	}
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, itemList)
	}
}

func addItem(c echo.Context) error {
	data, error := os.ReadFile(itemsJson)
	if error != nil {
		getErrorStatus(c, "Notfound items.json")
	}

	var itemList ItemList
	newData := bytes.NewReader(data)
	if error := json.NewDecoder(newData).Decode(&itemList); error != nil {
		getErrorStatus(c, "Failed to newReader items.json")
	}

	name := c.FormValue("name")
	category := c.FormValue("category")
	hashedImage := getHashedImage(c)

	newItem := Item{Name: name, Category: category, Image: hashedImage}

	itemList.Items = append(itemList.Items, newItem)

	updatedData, err := json.Marshal(itemList)
	if err != nil {
		getErrorStatus(c, "Failed to marshal items.json")
	}

	if err := os.WriteFile(itemsJson, updatedData, 0644); err != nil {
		getErrorStatus(c, "Failed to write items.json")
	}

	message := fmt.Sprintf("item received: %s", name)
	res := Response{Message: message}

	return c.JSON(http.StatusOK, res)
}

func getItemById(c echo.Context) error {
	id := c.Param("id")
	data, err := os.ReadFile(itemsJson)
	if err != nil {
		getErrorStatus(c, "Failed to read items.json")
	}

	var itemList ItemList
	newData := bytes.NewReader(data)
	if error := json.NewDecoder(newData).Decode(&itemList); error != nil {
		getErrorStatus(c,"Failed to newDecoder items.json")
	}

	for _, item := range itemList.Items {
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

	// Start server
	e.Logger.Fatal(e.Start(":9000"))
}
