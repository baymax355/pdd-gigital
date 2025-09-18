package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "path/filepath"
    "sync"

    "github.com/gin-gonic/gin"
)

const loginCookieName = "pdd_user"

type userStore struct {
    mu    sync.RWMutex
    users map[string]string // username -> password (plain text per requirements)
}

var usersDB = &userStore{users: map[string]string{}}

// loadUsers loads users from cfg.UsersFile.
func loadUsers() error {
    path := cfg.UsersFile
    if path == "" {
        return fmt.Errorf("UsersFile 未配置")
    }
    // Ensure directory exists for default path
    _ = os.MkdirAll(filepath.Dir(path), 0o755)
    data, err := os.ReadFile(path)
    if err != nil {
        if os.IsNotExist(err) {
            // If file not exists, keep empty map
            usersDB.mu.Lock()
            usersDB.users = map[string]string{}
            usersDB.mu.Unlock()
            return nil
        }
        return err
    }
    // Support two formats: {"username":"pwd", ...} or [{"username":"..","password":".."}]
    m := map[string]string{}
    if err := json.Unmarshal(data, &m); err == nil && len(m) > 0 {
        usersDB.mu.Lock()
        usersDB.users = m
        usersDB.mu.Unlock()
        return nil
    }
    // try array format
    var arr []map[string]string
    if err := json.Unmarshal(data, &arr); err == nil && len(arr) > 0 {
        for _, it := range arr {
            u := it["username"]
            p := it["password"]
            if u != "" {
                m[u] = p
            }
        }
        usersDB.mu.Lock()
        usersDB.users = m
        usersDB.mu.Unlock()
        return nil
    }
    return fmt.Errorf("无法解析用户文件: %s", path)
}

func (s *userStore) authenticate(username, password string) bool {
    s.mu.RLock()
    defer s.mu.RUnlock()
    if pwd, ok := s.users[username]; ok {
        return pwd == password
    }
    return false
}

func (s *userStore) listUsernames() []string {
    s.mu.RLock()
    defer s.mu.RUnlock()
    names := make([]string, 0, len(s.users))
    for name := range s.users {
        names = append(names, name)
    }
    return names
}

// GET /api/auth/me -> {username}
func handleAuthMe(c *gin.Context) {
    if u := usernameFromContext(c); u != "" {
        c.JSON(200, gin.H{"username": u})
        return
    }
    c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
}

// GET /api/auth/users -> {users: ["..", ".."]}
func handleAuthUsers(c *gin.Context) {
    c.JSON(200, gin.H{"users": usersDB.listUsernames()})
}

// POST /api/auth/login {username, password}
func handleAuthLogin(c *gin.Context) {
    var req struct {
        Username string `json:"username"`
        Password string `json:"password"`
    }
    if err := c.BindJSON(&req); err != nil {
        // also support form
        req.Username = c.PostForm("username")
        req.Password = c.PostForm("password")
    }
    if req.Username == "" || req.Password == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "用户名或密码不能为空"})
        return
    }
    if !usersDB.authenticate(req.Username, req.Password) {
        // try reload from disk once to reflect external updates
        _ = loadUsers()
        if !usersDB.authenticate(req.Username, req.Password) {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
            return
        }
    }
    // set cookie 30 days
    c.SetCookie(loginCookieName, req.Username, 30*24*3600, "/", "", false, true)
    c.JSON(200, gin.H{"username": req.Username})
}

// POST /api/auth/logout
func handleAuthLogout(c *gin.Context) {
    c.SetCookie(loginCookieName, "", -1, "/", "", false, true)
    c.JSON(200, gin.H{"ok": true})
}

func usernameFromContext(c *gin.Context) string {
    if v, err := c.Cookie(loginCookieName); err == nil && v != "" {
        return v
    }
    return ""
}
