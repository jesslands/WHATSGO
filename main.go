package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // Para decodificar PNG
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	_ "github.com/mattn/go-sqlite3"
)

type LineConfig struct {
	AllowCalls      bool   `json:"allow_calls"`
	RespondToGroups bool   `json:"respond_to_groups"`
	AutoMarkRead    bool   `json:"auto_mark_read"`
	AlwaysOnline    bool   `json:"always_online"`
	AutoReplyMsg    string `json:"auto_reply_msg"`
}

type Line struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Status     string            `json:"status"` // "disconnected", "qr_pending", "connected"
	QRCode     string            `json:"qr_code,omitempty"`
	Client     *whatsmeow.Client `json:"-"`
	WebhookURL string            `json:"webhook_url,omitempty"`
	Available  bool              `json:"available"`
	LastUsed   time.Time         `json:"last_used"`
	Config     LineConfig        `json:"config"`
	Active     bool              `json:"active"` // Si la línea está activa o pausada
}

type MessageRequest struct {
	From      string `json:"from,omitempty"`
	To        string `json:"to"`
	Message   string `json:"message"`
	MediaType string `json:"media_type,omitempty"` // "text", "image", "audio", "voice", "document", "video"
	MediaData string `json:"media_data,omitempty"` // Base64 encoded media
	FileName  string `json:"file_name,omitempty"`  // Nombre del archivo
	Caption   string `json:"caption,omitempty"`    // Caption para media
	MimeType  string `json:"mime_type,omitempty"`  // MIME type del archivo
}

type WebhookPayload struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Message string `json:"message"`
	LineID  string `json:"line_id"`
}

type WebhookConfig struct {
	LineID string `json:"line_id"`
	URL    string `json:"url"`
}

var (
	lines      = make(map[string]*Line)
	linesMutex sync.RWMutex
	container  *sqlstore.Container
	configDB   *sql.DB
)

func main() {
	// Crear directorio para almacenar sesiones
	os.MkdirAll("./sessions", 0755)
	os.MkdirAll("./public", 0755)

	// Inicializar base de datos de configuración
	var err error
	configDB, err = sql.Open("sqlite3", "./sessions/config.db")
	if err != nil {
		log.Fatalf("Error al abrir base de datos de configuración: %v", err)
	}
	defer configDB.Close()

	// Crear tabla de líneas si no existe
	err = initConfigDatabase()
	if err != nil {
		log.Fatalf("Error al inicializar base de datos: %v", err)
	}

	// Inicializar contenedor de base de datos de WhatsApp
	dbLog := waLog.Stdout("Database", "INFO", true)
	container, err = sqlstore.New(context.Background(), "sqlite3", "file:./sessions/whatsapp.db?_foreign_keys=on", dbLog)
	if err != nil {
		log.Fatalf("Error al crear contenedor de base de datos: %v", err)
	}

	// Cargar líneas existentes
	err = loadExistingLines()
	if err != nil {
		log.Printf("Advertencia al cargar líneas: %v", err)
	}

	router := mux.NewRouter()

	// API Endpoints
	api := router.PathPrefix("/api").Subrouter()
	api.HandleFunc("/lines", createLine).Methods("POST")
	api.HandleFunc("/lines", getLines).Methods("GET")
	api.HandleFunc("/lines/{id}", getLine).Methods("GET")
	api.HandleFunc("/lines/{id}/qr", getQRCode).Methods("GET")
	api.HandleFunc("/lines/{id}", deleteLine).Methods("DELETE")
	api.HandleFunc("/lines/{id}/webhook", setWebhook).Methods("POST")
	api.HandleFunc("/lines/{id}/config", updateLineConfig).Methods("PUT")
	api.HandleFunc("/lines/{id}/toggle", toggleLineActive).Methods("POST")
	api.HandleFunc("/lines/{id}/reconnect", reconnectLine).Methods("POST")
	api.HandleFunc("/messages/send", sendMessage).Methods("POST")
	api.HandleFunc("/messages/send-auto", sendMessageAuto).Methods("POST")
	api.HandleFunc("/stats", getStats).Methods("GET")

	// Servir archivos estáticos
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./public")))

	// CORS
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})

	handler := c.Handler(router)

	port := "12021"
	log.Printf("Servidor iniciado en http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}

// Inicializar base de datos de configuración
func initConfigDatabase() error {
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS lines (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		webhook_url TEXT,
		allow_calls BOOLEAN DEFAULT 0,
		respond_to_groups BOOLEAN DEFAULT 0,
		auto_mark_read BOOLEAN DEFAULT 1,
		always_online BOOLEAN DEFAULT 1,
		auto_reply_msg TEXT,
		active BOOLEAN DEFAULT 1,
		jid TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	
	CREATE TABLE IF NOT EXISTS message_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		line_id TEXT NOT NULL,
		direction TEXT NOT NULL, -- 'sent' o 'received'
		from_number TEXT NOT NULL,
		to_number TEXT NOT NULL,
		message_type TEXT NOT NULL, -- 'text', 'image', 'audio', 'video', 'document', 'voice'
		message_text TEXT,
		is_group BOOLEAN DEFAULT 0,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (line_id) REFERENCES lines(id) ON DELETE CASCADE
	);
	
	CREATE INDEX IF NOT EXISTS idx_message_logs_line_id ON message_logs(line_id);
	CREATE INDEX IF NOT EXISTS idx_message_logs_timestamp ON message_logs(timestamp);
	CREATE INDEX IF NOT EXISTS idx_message_logs_direction ON message_logs(direction);
	`
	_, err := configDB.Exec(createTableSQL)
	return err
}

// Guardar línea en base de datos
func saveLineToDB(line *Line) error {
	jid := ""
	if line.Client != nil && line.Client.Store.ID != nil {
		jid = line.Client.Store.ID.String()
	}

	query := `
	INSERT OR REPLACE INTO lines 
	(id, name, webhook_url, allow_calls, respond_to_groups, auto_mark_read, always_online, auto_reply_msg, active, jid, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`

	_, err := configDB.Exec(query,
		line.ID,
		line.Name,
		line.WebhookURL,
		line.Config.AllowCalls,
		line.Config.RespondToGroups,
		line.Config.AutoMarkRead,
		line.Config.AlwaysOnline,
		line.Config.AutoReplyMsg,
		line.Active,
		jid,
	)

	return err
}

// Eliminar línea de base de datos
func deleteLineFromDB(lineID string) error {
	_, err := configDB.Exec("DELETE FROM lines WHERE id = ?", lineID)
	return err
}

// Cargar líneas existentes desde la base de datos
func loadExistingLines() error {
	rows, err := configDB.Query(`
		SELECT id, name, webhook_url, allow_calls, respond_to_groups, 
		       auto_mark_read, always_online, auto_reply_msg, active, jid
		FROM lines
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, webhookURL, autoReplyMsg, jid string
		var allowCalls, respondToGroups, autoMarkRead, alwaysOnline, active bool

		err := rows.Scan(&id, &name, &webhookURL, &allowCalls, &respondToGroups,
			&autoMarkRead, &alwaysOnline, &autoReplyMsg, &active, &jid)
		if err != nil {
			log.Printf("Error al leer línea de DB: %v", err)
			continue
		}

		// Buscar dispositivo existente por JID
		deviceStore := container.NewDevice()
		if jid != "" {
			// Obtener todos los dispositivos
			devices, err := container.GetAllDevices(context.Background())
			if err != nil {
				log.Printf("Error al obtener dispositivos: %v", err)
			} else {
				// Buscar el dispositivo que coincida
				for _, dev := range devices {
					if dev.ID != nil && dev.ID.String() == jid {
						deviceStore = dev
						break
					}
				}
			}
		}

		clientLog := waLog.Stdout("Client-"+id, "INFO", true)
		client := whatsmeow.NewClient(deviceStore, clientLog)

		line := &Line{
			ID:         id,
			Name:       name,
			Status:     "disconnected",
			Client:     client,
			WebhookURL: webhookURL,
			Available:  false,
			Active:     active,
			Config: LineConfig{
				AllowCalls:      allowCalls,
				RespondToGroups: respondToGroups,
				AutoMarkRead:    autoMarkRead,
				AlwaysOnline:    alwaysOnline,
				AutoReplyMsg:    autoReplyMsg,
			},
		}

		// Configurar event handlers
		client.AddEventHandler(func(evt interface{}) {
			handleEvent(line, evt)
		})

		lines[id] = line

		// Intentar reconectar si tiene sesión y está activa
		if deviceStore.ID != nil && active {
			go connectLine(line)
			log.Printf("Línea %s cargada desde DB, reconectando...", id)
		} else if deviceStore.ID != nil && !active {
			log.Printf("Línea %s cargada desde DB (pausada)", id)
		} else {
			log.Printf("Línea %s cargada desde DB (sin sesión)", id)
		}
	}

	return rows.Err()
}

// Crear nueva línea
func createLine(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "El nombre es requerido", http.StatusBadRequest)
		return
	}

	linesMutex.Lock()
	defer linesMutex.Unlock()

	lineID := fmt.Sprintf("line_%d", time.Now().Unix())

	// Crear dispositivo
	deviceStore := container.NewDevice()
	clientLog := waLog.Stdout("Client-"+lineID, "INFO", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	line := &Line{
		ID:        lineID,
		Name:      req.Name,
		Status:    "disconnected",
		Client:    client,
		Available: false,
		Active:    true,
		Config: LineConfig{
			AllowCalls:      false,
			RespondToGroups: false,
			AutoMarkRead:    true,
			AlwaysOnline:    true,
			AutoReplyMsg:    "",
		},
	}

	// Configurar event handlers
	client.AddEventHandler(func(evt interface{}) {
		handleEvent(line, evt)
	})

	lines[lineID] = line

	// Guardar línea en base de datos
	err := saveLineToDB(line)
	if err != nil {
		log.Printf("Error al guardar línea en DB: %v", err)
	}

	// Intentar conectar
	go connectLine(line)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(line)
}

// Conectar línea
func connectLine(line *Line) {
	if line.Client.Store.ID == nil {
		// Nueva sesión - generar QR
		qrChan, _ := line.Client.GetQRChannel(context.Background())
		err := line.Client.Connect()
		if err != nil {
			log.Printf("Error al conectar cliente %s: %v", line.ID, err)
			return
		}

		line.Status = "qr_pending"

		for evt := range qrChan {
			if evt.Event == "code" {
				// Generar imagen QR
				png, err := qrcode.Encode(evt.Code, qrcode.Medium, 256)
				if err == nil {
					line.QRCode = fmt.Sprintf("data:image/png;base64,%s", encodeBase64(png))
				}
				log.Printf("Código QR generado para %s", line.ID)
			} else {
				log.Printf("Evento QR: %s", evt.Event)
			}
		}
	} else {
		// Sesión existente - reconectar
		err := line.Client.Connect()
		if err != nil {
			log.Printf("Error al reconectar cliente %s: %v", line.ID, err)
			return
		}
	}
}

// Manejar eventos de WhatsApp
func handleEvent(line *Line, rawEvt interface{}) {
	switch evt := rawEvt.(type) {
	case *events.Connected:
		line.Status = "connected"
		if line.Active {
			line.Available = true
		}
		line.QRCode = ""
		log.Printf("Línea %s conectada", line.ID)

		// Guardar JID en base de datos cuando se conecta por primera vez
		if line.Client.Store.ID != nil {
			go saveLineToDB(line)
		}

		// Configurar presencia según configuración
		if line.Config.AlwaysOnline && line.Active {
			go func() {
				line.Client.SendPresence(context.Background(), types.PresenceAvailable)
			}()
		}

	case *events.LoggedOut:
		line.Status = "disconnected"
		line.Available = false
		log.Printf("Línea %s desconectada", line.ID)

	case *events.Message:
		// Si la línea está desactivada, no procesar mensajes
		if !line.Active {
			return
		}

		// Marcar como leído según configuración
		if line.Config.AutoMarkRead {
			go func() {
				msgIDs := []types.MessageID{evt.Info.ID}
				line.Client.MarkRead(context.Background(), msgIDs, evt.Info.Timestamp, evt.Info.Chat, evt.Info.Sender)
			}()
		}

		// No responder a grupos según configuración
		if evt.Info.IsGroup && !line.Config.RespondToGroups {
			return
		}

		// Registrar mensaje recibido
		messageText := evt.Message.GetConversation()
		if messageText == "" && evt.Message.GetExtendedTextMessage() != nil {
			messageText = evt.Message.GetExtendedTextMessage().GetText()
		}

		messageType := "text"
		if evt.Message.GetImageMessage() != nil {
			messageType = "image"
		} else if evt.Message.GetAudioMessage() != nil {
			if evt.Message.GetAudioMessage().GetPTT() {
				messageType = "voice"
			} else {
				messageType = "audio"
			}
		} else if evt.Message.GetVideoMessage() != nil {
			messageType = "video"
		} else if evt.Message.GetDocumentMessage() != nil {
			messageType = "document"
		}

		go logMessage(line.ID, "received", evt.Info.Sender.String(), evt.Info.Chat.String(), messageType, messageText, evt.Info.IsGroup)

		// Enviar respuesta automática si está configurada
		if line.Config.AutoReplyMsg != "" {
			go func() {
				line.Client.SendMessage(context.Background(), evt.Info.Chat, &waProto.Message{
					Conversation: &line.Config.AutoReplyMsg,
				})
			}()
		}

		// Enviar a webhook si está configurado
		if line.WebhookURL != "" {
			go sendToWebhook(line, evt)
		}

	case *events.Receipt:
		// Manejar recibos de lectura
		if evt.Type == events.ReceiptTypeRead || evt.Type == events.ReceiptTypeReadSelf {
			log.Printf("Mensaje leído en línea %s", line.ID)
		}
	}
}

// Enviar a webhook
func sendToWebhook(line *Line, evt *events.Message) {
	payload := WebhookPayload{
		From:    evt.Info.Sender.String(),
		To:      evt.Info.Chat.String(),
		Message: evt.Message.GetConversation(),
		LineID:  line.ID,
	}

	if payload.Message == "" && evt.Message.GetExtendedTextMessage() != nil {
		payload.Message = evt.Message.GetExtendedTextMessage().GetText()
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error al serializar webhook: %v", err)
		return
	}

	resp, err := http.Post(line.WebhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Error al enviar webhook: %v", err)
		return
	}
	defer resp.Body.Close()

	log.Printf("Webhook enviado para línea %s", line.ID)
}

// Obtener todas las líneas
func getLines(w http.ResponseWriter, r *http.Request) {
	linesMutex.RLock()
	defer linesMutex.RUnlock()

	var result []*Line
	for _, line := range lines {
		lineCopy := &Line{
			ID:         line.ID,
			Name:       line.Name,
			Status:     line.Status,
			WebhookURL: line.WebhookURL,
			Available:  line.Available,
			LastUsed:   line.LastUsed,
			Config:     line.Config,
			Active:     line.Active,
		}
		result = append(result, lineCopy)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// Obtener una línea específica
func getLine(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	lineID := vars["id"]

	linesMutex.RLock()
	line, exists := lines[lineID]
	linesMutex.RUnlock()

	if !exists {
		http.Error(w, "Línea no encontrada", http.StatusNotFound)
		return
	}

	lineCopy := &Line{
		ID:         line.ID,
		Name:       line.Name,
		Status:     line.Status,
		WebhookURL: line.WebhookURL,
		Available:  line.Available,
		LastUsed:   line.LastUsed,
		Config:     line.Config,
		Active:     line.Active,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(lineCopy)
}

// Obtener código QR
func getQRCode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	lineID := vars["id"]

	linesMutex.RLock()
	line, exists := lines[lineID]
	linesMutex.RUnlock()

	if !exists {
		http.Error(w, "Línea no encontrada", http.StatusNotFound)
		return
	}

	response := map[string]string{
		"qr_code": line.QRCode,
		"status":  line.Status,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Eliminar línea
func deleteLine(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	lineID := vars["id"]

	linesMutex.Lock()
	defer linesMutex.Unlock()

	line, exists := lines[lineID]
	if !exists {
		http.Error(w, "Línea no encontrada", http.StatusNotFound)
		return
	}

	if line.Client != nil {
		line.Client.Disconnect()
	}

	delete(lines, lineID)

	// Eliminar línea de base de datos
	err := deleteLineFromDB(lineID)
	if err != nil {
		log.Printf("Error al eliminar línea de DB: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Línea eliminada"})
}

// Configurar webhook
func setWebhook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	lineID := vars["id"]

	var config WebhookConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	linesMutex.Lock()
	defer linesMutex.Unlock()

	line, exists := lines[lineID]
	if !exists {
		http.Error(w, "Línea no encontrada", http.StatusNotFound)
		return
	}

	line.WebhookURL = config.URL

	// Guardar línea en base de datos
	err := saveLineToDB(line)
	if err != nil {
		log.Printf("Error al guardar línea en DB: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Webhook configurado"})
}

// Actualizar configuración de línea
func updateLineConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	lineID := vars["id"]

	var newConfig LineConfig
	if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	linesMutex.Lock()
	defer linesMutex.Unlock()

	line, exists := lines[lineID]
	if !exists {
		http.Error(w, "Línea no encontrada", http.StatusNotFound)
		return
	}

	// Actualizar configuración
	line.Config = newConfig

	// Guardar línea en base de datos
	err := saveLineToDB(line)
	if err != nil {
		log.Printf("Error al guardar línea en DB: %v", err)
	}

	// Aplicar cambio de presencia si está conectada
	if line.Status == "connected" {
		if newConfig.AlwaysOnline {
			go func() {
				line.Client.SendPresence(context.Background(), types.PresenceAvailable)
			}()
		} else {
			go func() {
				line.Client.SendPresence(context.Background(), types.PresenceUnavailable)
			}()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Configuración actualizada",
		"config":  line.Config,
	})
}

// Activar/Desactivar línea
func toggleLineActive(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	lineID := vars["id"]

	var req struct {
		Active bool `json:"active"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	linesMutex.Lock()
	defer linesMutex.Unlock()

	line, exists := lines[lineID]
	if !exists {
		http.Error(w, "Línea no encontrada", http.StatusNotFound)
		return
	}

	line.Active = req.Active

	// Si se desactiva la línea, marcarla como no disponible
	if !req.Active {
		line.Available = false
		// Cambiar presencia a no disponible si está conectada
		if line.Status == "connected" {
			go func() {
				line.Client.SendPresence(context.Background(), types.PresenceUnavailable)
			}()
		}
	} else {
		// Si se activa y está conectada, marcarla como disponible
		if line.Status == "connected" {
			line.Available = true
			// Restaurar presencia según configuración
			if line.Config.AlwaysOnline {
				go func() {
					line.Client.SendPresence(context.Background(), types.PresenceAvailable)
				}()
			}
		}
	}

	// Guardar línea en base de datos
	err := saveLineToDB(line)
	if err != nil {
		log.Printf("Error al guardar línea en DB: %v", err)
	}

	status := "desactivada"
	if req.Active {
		status = "activada"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": fmt.Sprintf("Línea %s", status),
		"active":  line.Active,
	})
}

// Reconectar línea (generar nuevo QR o reconectar sesión existente)
func reconnectLine(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	lineID := vars["id"]

	linesMutex.Lock()
	defer linesMutex.Unlock()

	line, exists := lines[lineID]
	if !exists {
		http.Error(w, "Línea no encontrada", http.StatusNotFound)
		return
	}

	// Si ya está conectada, no hacer nada
	if line.Status == "connected" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "La línea ya está conectada",
			"status":  "connected",
		})
		return
	}

	// Desconectar cliente actual si existe
	if line.Client != nil {
		line.Client.Disconnect()
	}

	// Crear nuevo cliente
	deviceStore := container.NewDevice()
	clientLog := waLog.Stdout("Client-"+lineID, "INFO", true)
	newClient := whatsmeow.NewClient(deviceStore, clientLog)

	// Actualizar cliente
	line.Client = newClient
	line.Status = "disconnected"
	line.QRCode = ""
	line.Available = false

	// Configurar event handlers
	newClient.AddEventHandler(func(evt interface{}) {
		handleEvent(line, evt)
	})

	// Intentar conectar
	go connectLine(line)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Reconexión iniciada, generando nuevo código QR...",
		"status":  "qr_pending",
		"line_id": lineID,
	})
}

// Enviar mensaje con línea específica
func sendMessage(w http.ResponseWriter, r *http.Request) {
	var req MessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.From == "" || req.To == "" {
		http.Error(w, "From y To son requeridos", http.StatusBadRequest)
		return
	}

	linesMutex.RLock()
	line, exists := lines[req.From]
	linesMutex.RUnlock()

	if !exists {
		http.Error(w, "Línea no encontrada", http.StatusNotFound)
		return
	}

	if !line.Available || line.Status != "connected" {
		http.Error(w, "Línea no disponible", http.StatusServiceUnavailable)
		return
	}

	// Parsear número de destino
	recipient, err := parseJID(req.To)
	if err != nil {
		http.Error(w, "Número de destino inválido", http.StatusBadRequest)
		return
	}

	var msg *waProto.Message
	var uploadErr error

	// Determinar tipo de mensaje
	if req.MediaType != "" && req.MediaType != "text" {
		msg, uploadErr = createMediaMessage(line.Client, req)
		if uploadErr != nil {
			http.Error(w, fmt.Sprintf("Error al procesar media: %v", uploadErr), http.StatusBadRequest)
			return
		}
	} else {
		// Mensaje de texto simple
		if req.Message == "" {
			http.Error(w, "Message es requerido para mensajes de texto", http.StatusBadRequest)
			return
		}
		msg = &waProto.Message{
			Conversation: &req.Message,
		}
	}

	// Enviar mensaje
	_, err = line.Client.SendMessage(context.Background(), recipient, msg)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error al enviar mensaje: %v", err), http.StatusInternalServerError)
		return
	}

	line.LastUsed = time.Now()

	go logMessage(line.ID, "sent", line.Client.Store.ID.String(), req.To, req.MediaType, req.Message, false)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Mensaje enviado",
		"line_id": line.ID,
	})
}

// Enviar mensaje con línea automática
func sendMessageAuto(w http.ResponseWriter, r *http.Request) {
	var req MessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validar que al menos haya un mensaje o media
	if req.To == "" {
		http.Error(w, "To es requerido", http.StatusBadRequest)
		return
	}

	if req.Message == "" && req.MediaType == "" {
		http.Error(w, "Message o MediaType son requeridos", http.StatusBadRequest)
		return
	}

	linesMutex.RLock()
	defer linesMutex.RUnlock()

	// Buscar línea disponible
	var selectedLine *Line
	var oldestTime time.Time

	for _, line := range lines {
		if line.Available && line.Status == "connected" && line.Active {
			if selectedLine == nil || line.LastUsed.Before(oldestTime) {
				selectedLine = line
				oldestTime = line.LastUsed
			}
		}
	}

	if selectedLine == nil {
		http.Error(w, "No hay líneas disponibles", http.StatusServiceUnavailable)
		return
	}

	// Parsear número de destino
	recipient, err := parseJID(req.To)
	if err != nil {
		http.Error(w, "Número de destino inválido", http.StatusBadRequest)
		return
	}

	// Crear mensaje
	var msg *waProto.Message
	var uploadErr error

	if req.MediaType != "" && req.MediaType != "text" {
		// Es un mensaje multimedia
		msg, uploadErr = createMediaMessage(selectedLine.Client, req)
		if uploadErr != nil {
			http.Error(w, fmt.Sprintf("Error al procesar media: %v", uploadErr), http.StatusInternalServerError)
			return
		}
	} else {
		// Es un mensaje de texto
		msg = &waProto.Message{
			Conversation: &req.Message,
		}
	}

	// Enviar mensaje
	_, err = selectedLine.Client.SendMessage(context.Background(), recipient, msg)

	if err != nil {
		http.Error(w, fmt.Sprintf("Error al enviar mensaje: %v", err), http.StatusInternalServerError)
		return
	}

	selectedLine.LastUsed = time.Now()

	go logMessage(selectedLine.ID, "sent", selectedLine.Client.Store.ID.String(), req.To, req.MediaType, req.Message, false)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Mensaje enviado",
		"line_id": selectedLine.ID,
	})
}

// Utilidades

func parseJID(phone string) (types.JID, error) {
	// Limpiar número
	cleanPhone := ""
	for _, c := range phone {
		if c >= '0' && c <= '9' {
			cleanPhone += string(c)
		}
	}

	if len(cleanPhone) < 10 {
		return types.JID{}, fmt.Errorf("número inválido")
	}

	return types.NewJID(cleanPhone, types.DefaultUserServer), nil
}

// Procesar imagen para WhatsApp (optimizar y convertir si es necesario)
func processImageForWhatsApp(imageBytes []byte, mimeType string) ([]byte, string, error) {
	// Decodificar la imagen
	img, format, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, "", fmt.Errorf("error al decodificar imagen: %v", err)
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	log.Printf("Imagen detectada: formato=%s, dimensiones=%dx%d, tamaño=%d bytes", format, width, height, len(imageBytes))

	// Redimensionar si es muy grande (WhatsApp recomienda máximo 1280x1280)
	maxDimension := 1280
	needsResize := width > maxDimension || height > maxDimension

	var finalImg image.Image = img

	if needsResize {
		// Calcular nuevas dimensiones manteniendo aspect ratio
		var newWidth, newHeight int
		if width > height {
			newWidth = maxDimension
			newHeight = (height * maxDimension) / width
		} else {
			newHeight = maxDimension
			newWidth = (width * maxDimension) / height
		}

		log.Printf("Redimensionando imagen: %dx%d -> %dx%d", width, height, newWidth, newHeight)

		// Crear imagen redimensionada (simple nearest neighbor)
		resized := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
		for y := 0; y < newHeight; y++ {
			for x := 0; x < newWidth; x++ {
				srcX := (x * width) / newWidth
				srcY := (y * height) / newHeight
				resized.Set(x, y, img.At(srcX, srcY))
			}
		}
		finalImg = resized
	}

	// Convertir a JPEG con calidad optimizada
	var buf bytes.Buffer
	quality := 75 // Calidad base

	// Si el archivo original es muy grande, reducir calidad
	if len(imageBytes) > 500000 { // > 500KB
		quality = 60
	} else if len(imageBytes) > 200000 { // > 200KB
		quality = 70
	}

	opts := &jpeg.Options{Quality: quality}

	err = jpeg.Encode(&buf, finalImg, opts)
	if err != nil {
		return nil, "", fmt.Errorf("error al convertir imagen a JPEG: %v", err)
	}

	processedBytes := buf.Bytes()
	log.Printf("Imagen procesada: %d bytes -> %d bytes (JPEG calidad=%d)", len(imageBytes), len(processedBytes), quality)

	// Si todavía es muy grande, reducir calidad aún más
	if len(processedBytes) > 300000 {
		log.Printf("Imagen aún muy grande, reduciendo calidad a 50")
		buf.Reset()
		opts.Quality = 50
		err = jpeg.Encode(&buf, finalImg, opts)
		if err != nil {
			return nil, "", fmt.Errorf("error al recomprimir imagen: %v", err)
		}
		processedBytes = buf.Bytes()
		log.Printf("Imagen recomprimida: %d bytes (JPEG calidad=50)", len(processedBytes))
	}

	return processedBytes, "image/jpeg", nil
}

func createMediaMessage(client *whatsmeow.Client, req MessageRequest) (*waProto.Message, error) {
	// Limpiar y procesar media_data
	mediaData := req.MediaData
	mimeType := req.MimeType

	// Detectar y procesar Data URL (ej: data:image/png;base64,...)
	if len(mediaData) > 5 && mediaData[:5] == "data:" {
		// Buscar el final del prefijo (la coma)
		commaIndex := -1
		for i := 0; i < len(mediaData); i++ {
			if mediaData[i] == ',' {
				commaIndex = i
				break
			}
		}

		if commaIndex > 0 {
			prefix := mediaData[:commaIndex]

			// Extraer MIME type si no fue proporcionado
			if mimeType == "" {
				// Formato: data:image/png;base64
				// Extraer entre "data:" y ";"
				if len(prefix) > 5 {
					parts := prefix[5:] // Remover "data:"
					semicolonIndex := -1
					for i := 0; i < len(parts); i++ {
						if parts[i] == ';' {
							semicolonIndex = i
							break
						}
					}
					if semicolonIndex > 0 {
						mimeType = parts[:semicolonIndex]
						log.Printf("MIME type extraído del Data URL: %s", mimeType)
					}
				}
			}

			// Remover el prefijo, quedarnos solo con el base64
			mediaData = mediaData[commaIndex+1:]
			log.Printf("Prefijo Data URL removido, MIME: %s", mimeType)
		}
	}

	// Usar el MIME type extraído si no fue proporcionado
	if mimeType == "" && req.MimeType != "" {
		mimeType = req.MimeType
	}

	// Si no hay MIME type, intentar detectar por media_type
	if mimeType == "" {
		switch req.MediaType {
		case "image":
			mimeType = "image/jpeg"
		case "audio":
			mimeType = "audio/ogg; codecs=opus"
		case "voice":
			mimeType = "audio/ogg; codecs=opus" // Notas de voz en formato OGG Opus
		case "video":
			mimeType = "video/mp4"
		case "document":
			mimeType = "application/pdf"
		}
		log.Printf("MIME type por defecto asignado: %s", mimeType)
	}

	// Decodificar base64
	mediaBytes, err := base64.StdEncoding.DecodeString(mediaData)
	if err != nil {
		return nil, fmt.Errorf("error al decodificar base64: %v (data length: %d)", err, len(mediaData))
	}

	log.Printf("Archivo decodificado: %d bytes, MIME: %s, Tipo: %s", len(mediaBytes), mimeType, req.MediaType)

	// Validar tamaños recomendados
	maxSize := int64(0)
	switch req.MediaType {
	case "image":
		maxSize = 5 * 1024 * 1024 // 5 MB
	case "audio", "voice":
		maxSize = 16 * 1024 * 1024 // 16 MB
	case "video":
		maxSize = 16 * 1024 * 1024 // 16 MB
	case "document":
		maxSize = 100 * 1024 * 1024 // 100 MB
	}

	if int64(len(mediaBytes)) > maxSize {
		return nil, fmt.Errorf("archivo muy grande: %d bytes (máximo recomendado: %d bytes)", len(mediaBytes), maxSize)
	}

	// Procesar imagen si es necesario
	if req.MediaType == "image" {
		mediaBytes, mimeType, err = processImageForWhatsApp(mediaBytes, mimeType)
		if err != nil {
			return nil, fmt.Errorf("error al procesar imagen: %v", err)
		}
	}

	// Mapear tipo de media a tipo de WhatsApp correcto
	var whatsappMediaType whatsmeow.MediaType
	switch req.MediaType {
	case "image":
		whatsappMediaType = whatsmeow.MediaImage
	case "audio", "voice":
		whatsappMediaType = whatsmeow.MediaAudio
	case "video":
		whatsappMediaType = whatsmeow.MediaVideo
	case "document":
		whatsappMediaType = whatsmeow.MediaDocument
	default:
		return nil, fmt.Errorf("tipo de media no soportado: %s", req.MediaType)
	}

	// Subir archivo a WhatsApp
	uploaded, err := client.Upload(context.Background(), mediaBytes, whatsappMediaType)
	if err != nil {
		return nil, fmt.Errorf("error al subir archivo: %v (tamaño: %d bytes)", err, len(mediaBytes))
	}

	log.Printf("Archivo subido exitosamente: URL=%s, tamaño=%d bytes", uploaded.URL, len(mediaBytes))

	var msg *waProto.Message

	switch req.MediaType {
	case "image":
		msg = &waProto.Message{
			ImageMessage: &waProto.ImageMessage{
				Caption:       &req.Caption,
				URL:           &uploaded.URL,
				DirectPath:    &uploaded.DirectPath,
				MediaKey:      uploaded.MediaKey,
				Mimetype:      &mimeType,
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    &uploaded.FileLength,
			},
		}

	case "audio":
		ptt := false
		msg = &waProto.Message{
			AudioMessage: &waProto.AudioMessage{
				URL:           &uploaded.URL,
				DirectPath:    &uploaded.DirectPath,
				MediaKey:      uploaded.MediaKey,
				Mimetype:      &mimeType,
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    &uploaded.FileLength,
				PTT:           &ptt,
			},
		}

	case "voice":
		ptt := true // Nota de voz
		msg = &waProto.Message{
			AudioMessage: &waProto.AudioMessage{
				URL:           &uploaded.URL,
				DirectPath:    &uploaded.DirectPath,
				MediaKey:      uploaded.MediaKey,
				Mimetype:      &mimeType,
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    &uploaded.FileLength,
				PTT:           &ptt,
			},
		}

	case "video":
		msg = &waProto.Message{
			VideoMessage: &waProto.VideoMessage{
				Caption:       &req.Caption,
				URL:           &uploaded.URL,
				DirectPath:    &uploaded.DirectPath,
				MediaKey:      uploaded.MediaKey,
				Mimetype:      &mimeType,
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    &uploaded.FileLength,
			},
		}

	case "document":
		msg = &waProto.Message{
			DocumentMessage: &waProto.DocumentMessage{
				URL:           &uploaded.URL,
				DirectPath:    &uploaded.DirectPath,
				MediaKey:      uploaded.MediaKey,
				Mimetype:      &mimeType,
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    &uploaded.FileLength,
				FileName:      &req.FileName,
			},
		}

	default:
		return nil, fmt.Errorf("tipo de media no soportado: %s", req.MediaType)
	}

	return msg, nil
}

func encodeBase64(data []byte) string {
	const base64Table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	encoded := make([]byte, ((len(data)+2)/3)*4)

	j := 0
	for i := 0; i < len(data); i += 3 {
		b := (uint32(data[i]) << 16)
		if i+1 < len(data) {
			b |= uint32(data[i+1]) << 8
		}
		if i+2 < len(data) {
			b |= uint32(data[i+2])
		}

		encoded[j] = base64Table[(b>>18)&0x3F]
		encoded[j+1] = base64Table[(b>>12)&0x3F]
		encoded[j+2] = '='
		encoded[j+3] = '='

		if i+1 < len(data) {
			encoded[j+2] = base64Table[(b>>6)&0x3F]
		}
		if i+2 < len(data) {
			encoded[j+3] = base64Table[b&0x3F]
		}

		j += 4
	}

	return string(encoded)
}

// Registrar mensaje en la base de datos
func logMessage(lineID, direction, fromNumber, toNumber, messageType, messageText string, isGroup bool) error {
	query := `
	INSERT INTO message_logs 
	(line_id, direction, from_number, to_number, message_type, message_text, is_group)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	// Limitar el texto del mensaje a 500 caracteres para no hacer la BD muy grande
	if len(messageText) > 500 {
		messageText = messageText[:500] + "..."
	}

	_, err := configDB.Exec(query, lineID, direction, fromNumber, toNumber, messageType, messageText, isGroup)
	return err
}

// Obtener estadísticas
func getStats(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	lineID := r.URL.Query().Get("line_id")
	messageType := r.URL.Query().Get("message_type")

	// Default period: 30 days
	if period == "" {
		period = "30"
	}

	// Build WHERE clause
	whereConditions := []string{fmt.Sprintf("timestamp >= datetime('now', '-%s days')", period)}
	args := []interface{}{}

	if lineID != "" {
		whereConditions = append(whereConditions, "line_id = ?")
		args = append(args, lineID)
	}

	if messageType != "" {
		whereConditions = append(whereConditions, "message_type = ?")
		args = append(args, messageType)
	}

	whereClause := "WHERE " + whereConditions[0]
	for i := 1; i < len(whereConditions); i++ {
		whereClause += " AND " + whereConditions[i]
	}

	stats := make(map[string]interface{})

	// Overview stats
	overview := make(map[string]interface{})

	// Total messages
	var totalMessages int
	err := configDB.QueryRow("SELECT COUNT(*) FROM message_logs "+whereClause, args...).Scan(&totalMessages)
	if err != nil {
		log.Printf("Error getting total messages: %v", err)
	}
	overview["total_messages"] = totalMessages

	// Total sent
	var totalSent int
	sentArgs := append(args, "sent")
	err = configDB.QueryRow("SELECT COUNT(*) FROM message_logs "+whereClause+" AND direction = ?", sentArgs...).Scan(&totalSent)
	if err != nil {
		log.Printf("Error getting total sent: %v", err)
	}
	overview["total_sent"] = totalSent

	// Total received
	var totalReceived int
	receivedArgs := append(args, "received")
	err = configDB.QueryRow("SELECT COUNT(*) FROM message_logs "+whereClause+" AND direction = ?", receivedArgs...).Scan(&totalReceived)
	if err != nil {
		log.Printf("Error getting total received: %v", err)
	}
	overview["total_received"] = totalReceived

	// Total active lines
	linesMutex.RLock()
	activeLines := 0
	for _, line := range lines {
		if line.Active && line.Status == "connected" {
			activeLines++
		}
	}
	linesMutex.RUnlock()
	overview["total_lines"] = activeLines

	stats["overview"] = overview

	// Messages per day
	messagesPerDay := []map[string]interface{}{}
	query := `
	SELECT 
		DATE(timestamp) as date,
		SUM(CASE WHEN direction = 'sent' THEN 1 ELSE 0 END) as sent,
		SUM(CASE WHEN direction = 'received' THEN 1 ELSE 0 END) as received
	FROM message_logs
	` + whereClause + `
	GROUP BY DATE(timestamp)
	ORDER BY date ASC
	`
	rows, err := configDB.Query(query, args...)
	if err != nil {
		log.Printf("Error getting messages per day: %v", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var date string
			var sent, received int
			if err := rows.Scan(&date, &sent, &received); err == nil {
				messagesPerDay = append(messagesPerDay, map[string]interface{}{
					"date":     date,
					"sent":     sent,
					"received": received,
				})
			}
		}
	}
	stats["messages_per_day"] = messagesPerDay

	// Lines usage (only sent messages)
	linesUsage := []map[string]interface{}{}
	query = `
	SELECT 
		l.name as line_name,
		COUNT(*) as count
	FROM message_logs m
	JOIN lines l ON m.line_id = l.id
	` + whereClause + ` AND direction = 'sent'
	GROUP BY l.name
	ORDER BY count DESC
	LIMIT 10
	`
	rows, err = configDB.Query(query, args...)
	if err != nil {
		log.Printf("Error getting lines usage: %v", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var lineName string
			var count int
			if err := rows.Scan(&lineName, &count); err == nil {
				linesUsage = append(linesUsage, map[string]interface{}{
					"line_name": lineName,
					"count":     count,
				})
			}
		}
	}
	stats["lines_usage"] = linesUsage

	// Message types
	messageTypes := []map[string]interface{}{}
	query = `
	SELECT 
		message_type,
		COUNT(*) as count
	FROM message_logs
	` + whereClause + `
	GROUP BY message_type
	ORDER BY count DESC
	`
	rows, err = configDB.Query(query, args...)
	if err != nil {
		log.Printf("Error getting message types: %v", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var msgType string
			var count int
			if err := rows.Scan(&msgType, &count); err == nil {
				messageTypes = append(messageTypes, map[string]interface{}{
					"type":  msgType,
					"count": count,
				})
			}
		}
	}
	stats["message_types"] = messageTypes

	// Hourly distribution
	hourlyDistribution := []map[string]interface{}{}
	query = `
	SELECT 
		CAST(strftime('%H', timestamp) AS INTEGER) as hour,
		COUNT(*) as count
	FROM message_logs
	` + whereClause + `
	GROUP BY hour
	ORDER BY hour ASC
	`
	rows, err = configDB.Query(query, args...)
	if err != nil {
		log.Printf("Error getting hourly distribution: %v", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var hour, count int
			if err := rows.Scan(&hour, &count); err == nil {
				hourlyDistribution = append(hourlyDistribution, map[string]interface{}{
					"hour":  hour,
					"count": count,
				})
			}
		}
	}
	stats["hourly_distribution"] = hourlyDistribution

	// Top contacts
	topContacts := []map[string]interface{}{}
	query = `
	SELECT 
		CASE 
			WHEN direction = 'sent' THEN to_number 
			ELSE from_number 
		END as contact,
		SUM(CASE WHEN direction = 'sent' THEN 1 ELSE 0 END) as sent,
		SUM(CASE WHEN direction = 'received' THEN 1 ELSE 0 END) as received,
		COUNT(*) as total
	FROM message_logs
	` + whereClause + `
	GROUP BY contact
	ORDER BY total DESC
	LIMIT 10
	`
	rows, err = configDB.Query(query, args...)
	if err != nil {
		log.Printf("Error getting top contacts: %v", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var contact string
			var sent, received, total int
			if err := rows.Scan(&contact, &sent, &received, &total); err == nil {
				topContacts = append(topContacts, map[string]interface{}{
					"contact":  contact,
					"sent":     sent,
					"received": received,
					"total":    total,
				})
			}
		}
	}
	stats["top_contacts"] = topContacts

	// Recent activity
	recentActivity := []map[string]interface{}{}
	query = `
	SELECT 
		m.direction,
		m.from_number,
		m.to_number,
		m.message_type,
		m.message_text,
		m.timestamp,
		l.name as line_name
	FROM message_logs m
	LEFT JOIN lines l ON m.line_id = l.id
	` + whereClause + `
	ORDER BY m.timestamp DESC
	LIMIT 20
	`
	rows, err = configDB.Query(query, args...)
	if err != nil {
		log.Printf("Error getting recent activity: %v", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var direction, fromNumber, toNumber, messageType, messageText, timestamp, lineName string
			var messageTextSQL sql.NullString
			if err := rows.Scan(&direction, &fromNumber, &toNumber, &messageType, &messageTextSQL, &timestamp, &lineName); err == nil {
				if messageTextSQL.Valid {
					messageText = messageTextSQL.String
				}
				contact := fromNumber
				if direction == "sent" {
					contact = toNumber
				}
				recentActivity = append(recentActivity, map[string]interface{}{
					"direction":    direction,
					"contact":      contact,
					"message_type": messageType,
					"message_text": messageText,
					"timestamp":    timestamp,
					"line_name":    lineName,
				})
			}
		}
	}
	stats["recent_activity"] = recentActivity

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
