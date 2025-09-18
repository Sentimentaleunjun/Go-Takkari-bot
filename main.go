package main

import (
    "database/sql"
    "fmt"
    "log"
    "os"
    "os/signal"
    "strings"
    "sync"
    "syscall"
    "time"

    "github.com/bwmarrin/discordgo"
    _ "github.com/mattn/go-sqlite3"
)

var (
    token            = os.Getenv("DISCORD_TOKEN")
    dbPath           = "db.sqlite"
    mutex            sync.Mutex
    adminRoles       = []string{"1416769282380922991", "1416769872284876931"}
    patchnoteChannel = "1417426181942153237"
    supportChannel   = "1417052678378360914"
)

// -----------------------------
// DB 초기화
func initDB() {
    db := openDB()
    defer db.Close()

    stmts := []string{
        `CREATE TABLE IF NOT EXISTS patchnotes (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            title TEXT,
            content TEXT,
            author TEXT,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );`,
        `CREATE TABLE IF NOT EXISTS support (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            user_id TEXT,
            message TEXT,
            status TEXT DEFAULT 'open',
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );`,
        `CREATE TABLE IF NOT EXISTS schedule (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            title TEXT,
            description TEXT,
            date TEXT
        );`,
    }

    for _, stmt := range stmts {
        _, err := db.Exec(stmt)
        if err != nil {
            log.Fatal("DB 초기화 실패:", err)
        }
    }
    log.Println("✅ DB 초기화 완료")
}

func openDB() *sql.DB {
    db, err := sql.Open("sqlite3", dbPath)
    if err != nil {
        log.Fatal("DB 연결 실패:", err)
    }
    return db
}

// -----------------------------
// 헬퍼
func isAdmin(roles []string) bool {
    for _, r := range roles {
        for _, a := range adminRoles {
            if r == a {
                return true
            }
        }
    }
    return false
}

// -----------------------------
// 패치노트
func addPatchnote(title, content, author string) {
    mutex.Lock()
    defer mutex.Unlock()

    db := openDB()
    defer db.Close()

    _, err := db.Exec("INSERT INTO patchnotes (title, content, author) VALUES (?, ?, ?)", title, content, author)
    if err != nil {
        log.Println("패치노트 추가 실패:", err)
    }
}

func listPatchnotes() string {
    db := openDB()
    defer db.Close()

    rows, err := db.Query("SELECT id, title, content, author, created_at FROM patchnotes ORDER BY id DESC LIMIT 5")
    if err != nil {
        return "패치노트 조회 실패"
    }
    defer rows.Close()

    var result strings.Builder
    for rows.Next() {
        var id int
        var title, content, author string
        var createdAt string
        rows.Scan(&id, &title, &content, &author, &createdAt)
        result.WriteString(fmt.Sprintf("ID: %d\n제목: %s\n작성자: %s\n날짜: %s\n내용: %s\n\n", id, title, author, createdAt, content))
    }
    return result.String()
}

// 자동 채널 전송
func sendPatchnoteToChannel(s *discordgo.Session) {
    db := openDB()
    defer db.Close()

    rows, err := db.Query("SELECT title, content, author, created_at FROM patchnotes ORDER BY id DESC LIMIT 1")
    if err != nil {
        return
    }
    defer rows.Close()

    for rows.Next() {
        var title, content, author, createdAt string
        rows.Scan(&title, &content, &author, &createdAt)
        embed := &discordgo.MessageEmbed{
            Title:       title,
            Description: content,
            Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("작성자: %s | 날짜: %s", author, createdAt)},
            Color:       0x00ff00,
        }
        s.ChannelMessageSendEmbed(patchnoteChannel, embed)
    }
}

// -----------------------------
// 지원 티켓
func addSupport(userID, message string) {
    mutex.Lock()
    defer mutex.Unlock()

    db := openDB()
    defer db.Close()
    _, err := db.Exec("INSERT INTO support (user_id, message) VALUES (?, ?)", userID, message)
    if err != nil {
        log.Println("지원 티켓 등록 실패:", err)
    }
}

func listSupports() string {
    db := openDB()
    defer db.Close()

    rows, err := db.Query("SELECT id, user_id, message, status, created_at FROM support ORDER BY id DESC LIMIT 10")
    if err != nil {
        return "지원 티켓 조회 실패"
    }
    defer rows.Close()

    var result strings.Builder
    for rows.Next() {
        var id int
        var userID, message, status, createdAt string
        rows.Scan(&id, &userID, &message, &status, &createdAt)
        result.WriteString(fmt.Sprintf("ID: %d\n사용자: %s\n메시지: %s\n상태: %s\n등록: %s\n\n", id, userID, message, status, createdAt))
    }
    return result.String()
}

// -----------------------------
// 스케줄
func addSchedule(title, description, date string) {
    mutex.Lock()
    defer mutex.Unlock()

    db := openDB()
    defer db.Close()
    _, err := db.Exec("INSERT INTO schedule (title, description, date) VALUES (?, ?, ?)", title, description, date)
    if err != nil {
        log.Println("스케줄 등록 실패:", err)
    }
}

func listSchedules() string {
    db := openDB()
    defer db.Close()

    rows, err := db.Query("SELECT id, title, description, date FROM schedule ORDER BY id DESC LIMIT 5")
    if err != nil {
        return "스케줄 조회 실패"
    }
    defer rows.Close()

    var result strings.Builder
    for rows.Next() {
        var id int
        var title, description, date string
        rows.Scan(&id, &title, &description, &date)
        result.WriteString(fmt.Sprintf("ID: %d\n제목: %s\n설명: %s\n날짜: %s\n\n", id, title, description, date))
    }
    return result.String()
}

// -----------------------------
// Presence 업데이트
func startPresenceLoop(s *discordgo.Session) {
    go func() {
        for {
            guildCount := len(s.State.Guilds)
            memberCount := 0
            for _, g := range s.State.Guilds {
                memberCount += g.MemberCount
            }
            statuses := []string{
                fmt.Sprintf("%d개의 서버에서 활동중 ✨", guildCount),
                fmt.Sprintf("%d명의 유저와 함께 👥", memberCount),
                "따까리봇 업데이트 진행중",
            }

            for _, status := range statuses {
                s.UpdateGameStatus(0, status)
                time.Sleep(10 * time.Second)
            }
        }
    }()
}

// -----------------------------
// 명령어 등록
func registerCommands(dg *discordgo.Session) {
    dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        if i.Type != discordgo.InteractionApplicationCommand {
            return
        }

        switch i.ApplicationCommandData().Name {
        case "add_patchnote":
            if !isAdmin(i.Member.Roles) {
                s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
                    Type: discordgo.InteractionResponseChannelMessageWithSource,
                    Data: &discordgo.InteractionResponseData{
                        Content: "❌ 권한이 없습니다",
                        Flags:   1 << 6,
                    },
                })
                return
            }
            options := i.ApplicationCommandData().Options
            title := options[0].StringValue()
            content := options[1].StringValue()
            author := i.Member.User.Username
            addPatchnote(title, content, author)
            sendPatchnoteToChannel(s)
            s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
                Type: discordgo.InteractionResponseChannelMessageWithSource,
                Data: &discordgo.InteractionResponseData{
                    Content: "✅ 패치노트 등록 완료",
                },
            })

        case "view_patchnotes":
            notes := listPatchnotes()
            if notes == "" {
                notes = "패치노트가 없습니다"
            }
            s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
                Type: discordgo.InteractionResponseChannelMessageWithSource,
                Data: &discordgo.InteractionResponseData{
                    Content: notes,
                },
            })

        case "add_support":
            options := i.ApplicationCommandData().Options
            message := options[0].StringValue()
            addSupport(i.Member.User.ID, message)
            s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
                Type: discordgo.InteractionResponseChannelMessageWithSource,
                Data: &discordgo.InteractionResponseData{
                    Content: "✅ 지원 티켓 등록 완료",
                    Flags:   1 << 6,
                },
            })

        case "view_supports":
            if !isAdmin(i.Member.Roles) {
                s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
                    Type: discordgo.InteractionResponseChannelMessageWithSource,
                    Data: &discordgo.InteractionResponseData{
                        Content: "❌ 권한이 없습니다",
                        Flags:   1 << 6,
                    },
                })
                return
            }
            tickets := listSupports()
            s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
                Type: discordgo.InteractionResponseChannelMessageWithSource,
                Data: &discordgo.InteractionResponseData{
                    Content: tickets,
                },
            })
        }
    })
}

// -----------------------------
// 메인
func main() {
    if token == "" {
        log.Fatal("DISCORD_TOKEN 환경변수가 없습니다")
    }

    initDB()

    dg, err := discordgo.New("Bot " + token)
    if err != nil {