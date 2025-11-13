# WhatsGO

[![Go Version](https://img.shields.io/badge/Go-1.24+-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-Supported-blue.svg)](https://www.docker.com/)

WhatsGO es una aplicación de código abierto escrita en Go que proporciona una API REST completa y una interfaz web intuitiva para gestionar múltiples líneas de WhatsApp. Utiliza la biblioteca `whatsmeow` para interactuar con WhatsApp Web, permitiendo enviar y recibir mensajes, gestionar conexiones y obtener estadísticas detalladas.

## 🚀 Características

- **Gestión de Múltiples Líneas**: Crea y administra varias cuentas de WhatsApp simultáneamente
- **API REST Completa**: Endpoints para todas las operaciones principales
- **Interfaz Web Moderna**: Panel de control intuitivo con Tailwind CSS
- **Envío de Mensajes Multimedia**: Soporte para texto, imágenes, audio, video y documentos
- **Webhooks**: Recibe notificaciones en tiempo real de mensajes entrantes
- **Estadísticas Avanzadas**: Gráficos y métricas detalladas de uso
- **Configuración Flexible**: Personaliza el comportamiento de cada línea
- **Docker Support**: Despliegue fácil con contenedores
- **Base de Datos SQLite**: Almacenamiento persistente de configuraciones y logs

## 📋 Requisitos

- Go 1.24 o superior
- SQLite3
- WhatsApp (para vincular líneas)

## 🛠️ Instalación

### Opción 1: Desde el código fuente

```bash
# Clona el repositorio
git clone https://github.com/tu-usuario/whatsgo.git
cd whatsgo

# Instala dependencias
go mod download

# Ejecuta la aplicación
go run main.go
```

La aplicación estará disponible en `http://localhost:12021`

### Opción 2: Usando Docker

```bash
# Construye la imagen
docker build -t whatsgo .

# Ejecuta el contenedor
docker run -p 12021:12021 -v /ruta/a/sesiones:/app/sessions whatsgo
```

### Opción 3: Docker Compose

```bash
# Ejecuta con docker-compose
docker-compose up -d
```

La aplicación estará disponible en `http://localhost:12021`

## 📖 Uso

### Primeros Pasos

1. **Accede a la interfaz web** en `http://localhost:12021`
2. **Crea una nueva línea** haciendo clic en "Registrar Nueva Línea"
3. **Escanea el código QR** con WhatsApp para vincular la línea
4. **Configura webhooks** (opcional) para recibir mensajes entrantes
5. **Envía mensajes** usando la API o la interfaz web

### Configuración de Líneas

Cada línea puede configurarse con:
- **Permitir Llamadas**: Activar/desactivar recepción de llamadas
- **Responder a Grupos**: Procesar mensajes de grupos
- **Marcar como Leído**: Marcar mensajes automáticamente
- **Siempre en Línea**: Mantener presencia online
- **Respuesta Automática**: Mensaje automático para mensajes entrantes

## 🔌 API Reference

La API REST está disponible bajo el prefijo `/api`. Todos los endpoints devuelven JSON.

### Gestión de Líneas

#### Crear Línea
```http
POST /api/lines
Content-Type: application/json

{
  "name": "Línea Ventas"
}
```

**Respuesta:**
```json
{
  "id": "line_1234567890",
  "name": "Línea Ventas",
  "status": "disconnected",
  "available": false,
  "active": true,
  "config": {
    "allow_calls": false,
    "respond_to_groups": false,
    "auto_mark_read": true,
    "always_online": true,
    "auto_reply_msg": ""
  }
}
```

#### Obtener Todas las Líneas
```http
GET /api/lines
```

#### Obtener Línea Específica
```http
GET /api/lines/{id}
```

#### Obtener Código QR
```http
GET /api/lines/{id}/qr
```

**Respuesta:**
```json
{
  "qr_code": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAA...",
  "status": "qr_pending"
}
```

#### Eliminar Línea
```http
DELETE /api/lines/{id}
```

#### Configurar Webhook
```http
POST /api/lines/{id}/webhook
Content-Type: application/json

{
  "url": "https://tu-servidor.com/webhook"
}
```

#### Actualizar Configuración
```http
PUT /api/lines/{id}/config
Content-Type: application/json

{
  "allow_calls": true,
  "respond_to_groups": false,
  "auto_mark_read": true,
  "always_online": true,
  "auto_reply_msg": "Gracias por tu mensaje"
}
```

#### Activar/Desactivar Línea
```http
POST /api/lines/{id}/toggle
Content-Type: application/json

{
  "active": true
}
```

#### Reconectar Línea
```http
POST /api/lines/{id}/reconnect
```

### Envío de Mensajes

#### Enviar con Línea Específica
```http
POST /api/messages/send
Content-Type: application/json

{
  "from": "line_1234567890",
  "to": "521234567890",
  "message": "Hola, ¿cómo estás?",
  "media_type": "text"
}
```

#### Enviar Automáticamente
```http
POST /api/messages/send-auto
Content-Type: application/json

{
  "to": "521234567890",
  "message": "Mensaje automático",
  "media_type": "text"
}
```

**Tipos de Media Soportados:**
- `text`: Mensaje de texto
- `image`: Imagen (base64)
- `audio`: Archivo de audio
- `voice`: Nota de voz
- `video`: Video
- `document`: Documento

**Ejemplo con Imagen:**
```json
{
  "to": "521234567890",
  "media_type": "image",
  "media_data": "data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAAAQ...",
  "caption": "Foto de ejemplo"
}
```

### Estadísticas

#### Obtener Estadísticas
```http
GET /api/stats?period=30&line_id=line_123&message_type=text
```

**Parámetros de consulta:**
- `period`: Días (7, 15, 30, 60, 90)
- `line_id`: ID de línea específica
- `message_type`: Tipo de mensaje

**Respuesta:**
```json
{
  "overview": {
    "total_messages": 1250,
    "total_sent": 680,
    "total_received": 570,
    "total_lines": 3
  },
  "messages_per_day": [...],
  "lines_usage": [...],
  "message_types": [...],
  "hourly_distribution": [...],
  "top_contacts": [...],
  "recent_activity": [...]
}
```

## 🌐 Interfaz Web

WhatsGO incluye una interfaz web completa en el directorio `public/` que consume la API REST. La interfaz proporciona:

### Panel de Líneas (`index.html`)
- **Vista general** de todas las líneas registradas
- **Creación de líneas** con nombres personalizados
- **Escaneo de QR** para vinculación con WhatsApp
- **Configuración de webhooks** para notificaciones
- **Gestión de estado** (activar/desactivar líneas)
- **Configuración avanzada** por línea

### Envío de Mensajes (`send.html`)
- **Envío manual** especificando línea y destinatario
- **Envío automático** (selección automática de línea disponible)
- **Soporte multimedia** con preview de archivos
- **Historial de envíos** recientes

### Estadísticas (`stats.html`)
- **Métricas generales** (totales, líneas activas)
- **Gráficos interactivos** con Chart.js
- **Filtros avanzados** por período, línea y tipo
- **Contactos más activos**
- **Actividad reciente**

## 🔧 Configuración Avanzada

### Variables de Entorno
- `PORT`: Puerto del servidor (default: 12021)

### Base de Datos
- **Configuración**: `./sessions/config.db`
- **Sesiones WhatsApp**: `./sessions/whatsapp.db`

### Webhooks
Los webhooks envían POST requests con el siguiente formato:
```json
{
  "from": "521234567890@s.whatsapp.net",
  "to": "521234567890@s.whatsapp.net",
  "message": "Contenido del mensaje",
  "line_id": "line_1234567890"
}
```

## 🤝 Contribución

¡Las contribuciones son bienvenidas! Este es un proyecto open source y apreciamos cualquier ayuda.

### Cómo contribuir:
1. **Fork** el repositorio
2. **Crea una rama** para tu feature (`git checkout -b feature/nueva-funcionalidad`)
3. **Commit** tus cambios (`git commit -am 'Agrega nueva funcionalidad'`)
4. **Push** a la rama (`git push origin feature/nueva-funcionalidad`)
5. **Crea un Pull Request**

## 📄 Licencia

Este proyecto está bajo la Licencia MIT. Ver el archivo [LICENSE](LICENSE) para más detalles.

## ⚠️ Aviso Legal

Este proyecto es para fines educativos y de automatización. El uso de WhatsApp debe cumplir con sus términos de servicio. Los desarrolladores no se hacen responsables del mal uso de esta herramienta.

