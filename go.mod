module github.com/kong/botdiscord

go 1.26.2

require (
	github.com/bwmarrin/discordgo v0.29.1-0.20251229154532-54ae40de5723
	github.com/joho/godotenv v1.5.1
)

require (
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	golang.org/x/crypto v0.32.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
)

replace github.com/bwmarrin/discordgo => github.com/yeongaori/discordgo-fork v0.0.0-20260616160332-4a325f170f70
