package main

import (
	"bytes"
	"crypto/sha256"
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

func parseError(c echo.Context, message string, err error) {
	res := Response{Message: message}
	c.JSON(http.StatusInternalServerError, res);
	c.Logger().Error(err);
}

func root(c echo.Context) error {
	res := Response{Message: "Hello, world!"}
	return c.JSON(http.StatusOK, res)
}

func getItems(c echo.Context) error {
	data, err := os.ReadFile(itemsJson)
	if err != nil {
		parseError(c, "Failed to read items.json", err)
		return err
	}
	return c.JSONBlob(http.StatusOK, data)
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

	return hashedImage ,nil
}

func addItem(c echo.Context) error {
	data, error := os.ReadFile(itemsJson)
	if error != nil {
		parseError(c, "Notfound items.json", error)
		return error
	}

	var items Items
	newData := bytes.NewReader(data)
	if error := json.NewDecoder(newData).Decode(&items); error != nil {
		parseError(c, "Failed to newReader items.json", error)
		return error
	}

	name := c.FormValue("name")
	category := c.FormValue("category")
	hashedImage, error := getHashedImage(c)
	if error != nil {
		parseError(c, "Failed to get hashed image", error)
		return error
	}

	newItem := Item{Name: name, Category: category, Image: hashedImage}

	items.Items = append(items.Items, &newItem)

	updatedData, error := json.Marshal(items)
	if error != nil {
		parseError(c, "Failed to marshal items.json", error)
		return error
	}

	if error := os.WriteFile(itemsJson, updatedData, 0644); error != nil {
		parseError(c, "Failed to write items.json", error)
		return error
	}

	message := fmt.Sprintf("item received: %s", name)
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
