package main

import (
	"crypto/sha256"
	"database/sql"
	"errors"
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
	ImgDir               = "images"
	DBPath               = "./db/mercari.sqlite3"
	ItemsSchemaPath      = "./db/items.db"
	CategoriesSchemaPath = "./db/categories.db"
)

type Response struct {
	Message string `json:"message"`
}

type Item struct {
	Id       int    `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Image    string `json:"image_name"`
}

type Items struct {
	Items []Item `json:"items"`
}

type ServerImpl struct {
	db *sql.DB
}

// TODO:DBの初期化はアプリケーションロジックと分けて実施するのが一般的なので調査する
func (s ServerImpl) createTables() error {
	// スキーマを読み込む
	itemsSchema, err := os.ReadFile(ItemsSchemaPath)
	if err != nil {
		return fmt.Errorf("failed to read items schema: %w", err)
	}
	categoriesSchema, err := os.ReadFile(CategoriesSchemaPath)
	if err != nil {
		return fmt.Errorf("failed to read categories schema: %w", err)
	}

	// テーブルがない場合は作成
	if _, err := s.db.Exec(string(categoriesSchema)); err != nil {
		return fmt.Errorf("failed to create categories table: %w", err)
	}
	if _, err := s.db.Exec(string(itemsSchema)); err != nil {
		return fmt.Errorf("failed to create items table: %w", err)
	}

	return nil
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
	imageFile, err := c.FormFile("image")
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

func (s ServerImpl) getItems(c echo.Context) error {
	rows, err := s.db.Query("SELECT items.id, items.name, categories.name, items.image_name FROM items JOIN categories ON items.category_id = categories.id;")
	if err != nil {
		parseError(c, "Failed to get items from DB", err)
		return err
	}
	defer rows.Close()

	items := Items{Items: []Item{}}
	for rows.Next() {
		var item Item
		err = rows.Scan(&item.Id, &item.Name, &item.Category, &item.Image)
		if err != nil {
			parseError(c, "Failed to scan rows", err)
			return err
		}
		items.Items = append(items.Items, item)
	}
	return c.JSON(http.StatusOK, items)
}

func (s ServerImpl) addItem(c echo.Context) error {
	name := c.FormValue("name")
	category := c.FormValue("category")

	hashedImage, err := getHashedImage(c)
	if err != nil {
		parseError(c, "Failed to get hashed image", err)
		return err
	}

	// トランザクション開始
	tx, err := s.db.Begin()
	if err != nil {
		c.Logger().Errorf("Error starting database transaction: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Error starting database transaction")
	}
	// トランザクション内でエラーが発生した場合、ロールバックすることでデータベースの状態を元に戻す
	defer tx.Rollback()

	var categoryID int64
	// カテゴリ名に対応するカテゴリIDを取得
	err = tx.QueryRow("SELECT id FROM categories WHERE name = ?", category).Scan(&categoryID)

	// カテゴリが存在しない場合、新しいカテゴリを追加
	if errors.Is(err, sql.ErrNoRows) {
		result, err := tx.Exec("INSERT INTO categories (name) VALUES (?)", category)
		if err != nil {
			parseError(c, "Failed to insert category into database", err)
			return err
		}
		categoryID, _ = result.LastInsertId()
	} else if err != nil {
		parseError(c, "Failed to query category from the database", err)
		return err
	}

	// itemsテーブルに商品を追加
	statement, err := tx.Prepare("INSERT INTO items (name, category_id, image_name) VALUES (?, ?, ?)")
	if err != nil {
		parseError(c, "Failed to prepare statement for database insertion", err)
	}
	defer statement.Close()

	_, err = statement.Exec(name, categoryID, hashedImage)
	if err != nil {
		c.Logger().Errorf("Error saving item to database: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Error saving item to database")
	}

	// トランザクションをコミットすると、トランザクション内で行った変更がデータベースに反映される
	if err := tx.Commit(); err != nil {
		parseError(c, "Failed to commit transaction", err)
		return err
	}

	// ログとJSONレスポンスの作成
	c.Logger().Infof("Received item: %s, Category: %s", name, category)
	message := fmt.Sprintf("item received: %s", name)
	res := Response{Message: message}

	return c.JSON(http.StatusOK, res)
}

func (s ServerImpl) getItemById(c echo.Context) error {
	id := c.Param("id")
	var item Item
	query := "SELECT name, category_id, image_name FROM items WHERE id = ?"
	err := s.db.QueryRow(query, id).Scan(&item.Name, &item.Category, &item.Image)
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

func (s ServerImpl) searchItems(c echo.Context) error {
	keyword := c.QueryParam("keyword")

	query := `SELECT name, category_id FROM items WHERE name LIKE ?`
	rows, err := s.db.Query(query, "%"+keyword+"%")
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
		e.Logger.Errorf("Failed to open database: %v", err)
		return
	}
	defer db.Close()

	serverImpl := ServerImpl{db: db}

	if err := serverImpl.createTables(); err != nil {
		e.Logger.Errorf("Failed to create tables: %v", err)
		return
	}

	// Routes
	e.GET("/", root)
	e.POST("/items", serverImpl.addItem)
	e.GET("/items", serverImpl.getItems)
	e.GET("/items/:id", serverImpl.getItemById)
	e.GET("/image/:imageFilename", getImg)
	e.GET("/search", serverImpl.searchItems)

	// Start server
	e.Logger.Fatal(e.Start(":9000"))
}
