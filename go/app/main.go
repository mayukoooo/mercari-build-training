package main

import (
	"crypto/sha256"
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
	Name     string `json:"name"`
	Category string `json:"category"`
	Image    string `json:"image_name"`
}

type ItemList struct {
	Items []Item `json:"items"`
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

func addItem(c echo.Context) error {
	data, error := os.ReadFile(itemsJson)
	if error != nil {
		c.Logger().Error("Notfound items.json")
	}

	var itemList ItemList
	if error := json.Unmarshal(data, &itemList); error != nil {
		c.Logger().Error("Failed to unmarshal items.json")
	}

	name := c.FormValue("name")
	category := c.FormValue("category")

	imageFile, error := c.FormFile("image")
	if error != nil {
		c.Logger().Error("Failed to get image file")
	}

	src, err := imageFile.Open()
	if err != nil {
		c.Logger().Error("Failed to open image file")
	}
	defer src.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, src); err != nil {
		c.Logger().Error("Failed to hash image file")
	}

	src.Seek(0, 0)
	hashed := fmt.Sprintf("%x", hash.Sum(nil)) + ".jpg"

	dst, err := os.Create(path.Join(ImgDir, hashed))
	if err != nil {
		c.Logger().Error("Failed to create image file")
	}

	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		c.Logger().Error("Failed to copy image file")
	}

	newItem := Item{Name: name, Category: category, Image: hashed}

	itemList.Items = append(itemList.Items, newItem)

	updatedData, err := json.Marshal(itemList)
	if err != nil {
		c.Logger().Error("Failed to marshal items.json")
	}

	if err := os.WriteFile(itemsJson, updatedData, 0644); err != nil {
		c.Logger().Error("Failed to write items.json")
	}

	message := fmt.Sprintf("item received: %s", name)
	res := Response{Message: message}

	return c.JSON(http.StatusOK, res)
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
	e.GET("/image/:imageFilename", getImg)

	// Start server
	e.Logger.Fatal(e.Start(":9000"))
}
