package main

import (
	"encoding/json"
	"fmt"
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

func addItem(c echo.Context) error {
	data, error := os.ReadFile(itemsJson)
	if error != nil {
		getErrorStatus(c, "Notfound items.json")
	}

	var itemList ItemList
	if error := json.Unmarshal(data, &itemList); error != nil {
		getErrorStatus(c, "Failed to unmarshal items.json")
	}

	name := c.FormValue("name")
	category := c.FormValue("category")
	newItem := Item{Name: name, Category: category}

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

	// http.StatusCreated(201) is also good choice.StatusOK
  // but in that case, you need to implement and return a URL
  //   that returns information on the posted item.
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

	frontURL := os.Getenv("FRONT_URL")
	if frontURL == "" {
		frontURL = "http://localhost:3000"
	}
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{frontURL},
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
