// Detectar la URL base automáticamente
const API_BASE = window.location.origin + '/api';

let messageHistory = [];

// Límites de tamaño de archivo (en bytes)
const FILE_SIZE_LIMITS = {
    'image': 5 * 1024 * 1024,      // 5 MB
    'audio': 16 * 1024 * 1024,     // 16 MB
    'video': 16 * 1024 * 1024,     // 16 MB
    'document': 100 * 1024 * 1024  // 100 MB
};

// MIME types recomendados
const RECOMMENDED_MIMES = {
    'image': ['image/jpeg', 'image/png', 'image/gif', 'image/webp'],
    'audio': ['audio/ogg', 'audio/mpeg', 'audio/mp3', 'audio/aac'],
    'video': ['video/mp4', 'video/3gpp', 'video/quicktime'],
    'document': ['application/pdf', 'application/msword', 'application/vnd.openxmlformats-officedocument.wordprocessingml.document']
};

// Load lines on page load
document.addEventListener('DOMContentLoaded', () => {
    loadLinesForSelect();
    
    // Agregar event listeners para preview de archivos
    document.getElementById('mediaFile').addEventListener('change', handleFileSelect);
    document.getElementById('mediaFileAuto').addEventListener('change', handleFileSelectAuto);
});

// Load lines for select dropdown
async function loadLinesForSelect() {
    try {
        const response = await fetch(`${API_BASE}/lines`);
        if (!response.ok) {
            throw new Error('Error al cargar las líneas');
        }

        const lines = await response.json();
        const select = document.getElementById('lineSelect');
        
        select.innerHTML = '<option value="">Seleccionar línea...</option>';
        
        if (lines && lines.length > 0) {
            lines.forEach(line => {
                if (line.status === 'connected' && line.available) {
                    const option = document.createElement('option');
                    option.value = line.id;
                    option.textContent = `${line.name} (${line.id})`;
                    select.appendChild(option);
                }
            });
        }
    } catch (error) {
        console.error('Error:', error);
        showToast('Error al cargar las líneas', 'error');
    }
}

// Send message with specific line
async function sendMessageWithLine(event) {
    event.preventDefault();

    const lineId = document.getElementById('lineSelect').value;
    const to = document.getElementById('toNumber').value.trim();
    const messageType = document.getElementById('messageType').value;

    if (!lineId || !to) {
        showToast('Por favor completa los campos requeridos', 'error');
        return;
    }

    let payload = {
        from: lineId,
        to: to,
    };

    try {
        if (messageType === 'text') {
            const message = document.getElementById('message').value.trim();
            if (!message) {
                showToast('Por favor escribe un mensaje', 'error');
                return;
            }
            payload.message = message;
        } else {
            // Multimedia
            const file = document.getElementById('mediaFile').files[0];
            if (!file) {
                showToast('Por favor selecciona un archivo', 'error');
                return;
            }

            // Validar tamaño (máximo 16MB)
            if (file.size > 16 * 1024 * 1024) {
                showToast('El archivo es demasiado grande (máximo 16MB)', 'error');
                return;
            }

            // Convertir archivo a base64
            showToast('Procesando archivo...', 'info');
            const base64Data = await fileToBase64(file);
            
            payload.media_type = messageType;
            payload.media_data = base64Data;
            payload.mime_type = file.type;
            
            if (messageType === 'document') {
                payload.file_name = file.name;
            }
            
            const caption = document.getElementById('caption').value.trim();
            if (caption) {
                payload.caption = caption;
            }
        }

        const response = await fetch(`${API_BASE}/messages/send`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify(payload),
        });

        if (!response.ok) {
            const error = await response.text();
            throw new Error(error);
        }

        const result = await response.json();
        showToast('Mensaje enviado exitosamente', 'success');
        
        // Clear form
        document.getElementById('toNumber').value = '';
        document.getElementById('message').value = '';
        document.getElementById('mediaFile').value = '';
        document.getElementById('caption').value = '';
        document.getElementById('filePreview').classList.add('hidden');
        
        // Add to history
        addToHistory({
            to: to,
            message: messageType === 'text' ? payload.message : `[${messageType.toUpperCase()}]`,
            line: lineId,
            timestamp: new Date(),
        });
    } catch (error) {
        console.error('Error:', error);
        showToast(`Error al enviar mensaje: ${error.message}`, 'error');
    }
}

// Send message automatically
async function sendMessageAuto(event) {
    event.preventDefault();

    const to = document.getElementById('toNumberAuto').value.trim();
    const messageType = document.getElementById('messageTypeAuto').value;

    if (!to) {
        showToast('Por favor ingresa el número de destino', 'error');
        return;
    }

    let payload = {
        to: to,
    };

    try {
        if (messageType === 'text') {
            const message = document.getElementById('messageAuto').value.trim();
            if (!message) {
                showToast('Por favor escribe un mensaje', 'error');
                return;
            }
            payload.message = message;
        } else {
            // Multimedia
            const file = document.getElementById('mediaFileAuto').files[0];
            if (!file) {
                showToast('Por favor selecciona un archivo', 'error');
                return;
            }

            // Validar tamaño (máximo 16MB)
            if (file.size > 16 * 1024 * 1024) {
                showToast('El archivo es demasiado grande (máximo 16MB)', 'error');
                return;
            }

            // Convertir archivo a base64
            showToast('Procesando archivo...', 'info');
            const base64Data = await fileToBase64(file);
            
            payload.media_type = messageType;
            payload.media_data = base64Data;
            payload.mime_type = file.type;
            
            if (messageType === 'document') {
                payload.file_name = file.name;
            }
            
            const caption = document.getElementById('captionAuto').value.trim();
            if (caption) {
                payload.caption = caption;
            }
        }

        const response = await fetch(`${API_BASE}/messages/send-auto`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify(payload),
        });

        if (!response.ok) {
            const error = await response.text();
            throw new Error(error);
        }

        const result = await response.json();
        showToast(`Mensaje enviado por ${result.line_id}`, 'success');
        
        // Clear form
        document.getElementById('toNumberAuto').value = '';
        document.getElementById('messageAuto').value = '';
        document.getElementById('mediaFileAuto').value = '';
        document.getElementById('captionAuto').value = '';
        document.getElementById('filePreviewAuto').classList.add('hidden');
        
        // Add to history
        addToHistory({
            to: to,
            message: messageType === 'text' ? payload.message : `[${messageType.toUpperCase()}]`,
            line: result.line_id,
            timestamp: new Date(),
            auto: true,
        });
    } catch (error) {
        console.error('Error:', error);
        showToast(`Error al enviar mensaje: ${error.message}`, 'error');
    }
}

// Add to message history
function addToHistory(item) {
    messageHistory.unshift(item);
    
    // Keep only last 10 messages
    if (messageHistory.length > 10) {
        messageHistory = messageHistory.slice(0, 10);
    }
    
    displayHistory();
}

// Display message history
function displayHistory() {
    const container = document.getElementById('messageHistory');
    
    if (messageHistory.length === 0) {
        container.innerHTML = '<p class="text-gray-500 text-center py-8">No hay mensajes enviados aún</p>';
        return;
    }
    
    container.innerHTML = messageHistory.map(item => {
        const time = new Date(item.timestamp).toLocaleString('es-MX', {
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
            day: '2-digit',
            month: '2-digit',
        });
        
        return `
            <div class="border border-gray-200 rounded-lg p-4 hover:bg-gray-50 transition">
                <div class="flex items-start justify-between">
                    <div class="flex-1">
                        <div class="flex items-center space-x-2 mb-2">
                            <span class="text-sm font-semibold text-gray-700">Para: ${item.to}</span>
                            <span class="px-2 py-1 rounded-full text-xs font-medium ${item.auto ? 'bg-purple-100 text-purple-800' : 'bg-green-100 text-green-800'}">
                                ${item.auto ? 'Automático' : 'Manual'}
                            </span>
                        </div>
                        <p class="text-sm text-gray-600 mb-2">${item.message}</p>
                        <div class="flex items-center space-x-4 text-xs text-gray-500">
                            <span><i class="fas fa-phone-alt mr-1"></i>${item.line}</span>
                            <span><i class="fas fa-clock mr-1"></i>${time}</span>
                        </div>
                    </div>
                </div>
            </div>
        `;
    }).join('');
}

// Show Toast
function showToast(message, type = 'info') {
    const toast = document.getElementById('toast');
    const toastMessage = document.getElementById('toastMessage');

    const colors = {
        success: 'bg-green-600 text-white',
        error: 'bg-red-600 text-white',
        info: 'bg-blue-600 text-white'
    };

    toast.className = `fixed bottom-4 right-4 px-6 py-3 rounded-lg shadow-lg ${colors[type]}`;
    toastMessage.textContent = message;
    toast.classList.remove('hidden');

    setTimeout(() => {
        toast.classList.add('hidden');
    }, 3000);
}

// Toggle media input for specific line
function toggleMediaInput() {
    const messageType = document.getElementById('messageType').value;
    const textDiv = document.getElementById('textMessageDiv');
    const mediaDiv = document.getElementById('mediaInputDiv');
    const captionDiv = document.getElementById('captionDiv');
    const messageField = document.getElementById('message');

    if (messageType === 'text') {
        textDiv.classList.remove('hidden');
        mediaDiv.classList.add('hidden');
        captionDiv.classList.add('hidden');
        messageField.required = true;
    } else {
        textDiv.classList.add('hidden');
        mediaDiv.classList.remove('hidden');
        messageField.required = false;
        
        // Mostrar caption para imagen, video
        if (messageType === 'image' || messageType === 'video') {
            captionDiv.classList.remove('hidden');
        } else {
            captionDiv.classList.add('hidden');
        }
    }
}

// Toggle media input for auto send
function toggleMediaInputAuto() {
    const messageType = document.getElementById('messageTypeAuto').value;
    const textDiv = document.getElementById('textMessageDivAuto');
    const mediaDiv = document.getElementById('mediaInputDivAuto');
    const captionDiv = document.getElementById('captionDivAuto');
    const messageField = document.getElementById('messageAuto');

    if (messageType === 'text') {
        textDiv.classList.remove('hidden');
        mediaDiv.classList.add('hidden');
        captionDiv.classList.add('hidden');
        messageField.required = true;
    } else {
        textDiv.classList.add('hidden');
        mediaDiv.classList.remove('hidden');
        messageField.required = false;
        
        // Mostrar caption para imagen, video
        if (messageType === 'image' || messageType === 'video') {
            captionDiv.classList.remove('hidden');
        } else {
            captionDiv.classList.add('hidden');
        }
    }
}

// Handle file selection
function handleFileSelect(event) {
    const file = event.target.files[0];
    const messageType = document.getElementById('messageType').value;
    
    if (file) {
        // Validar tamaño
        const maxSize = FILE_SIZE_LIMITS[messageType];
        if (file.size > maxSize) {
            showToast(`Archivo muy grande. Máximo: ${formatFileSize(maxSize)}`, 'error');
            event.target.value = ''; // Limpiar selección
            return;
        }
        
        // Validar MIME type (advertencia, no error)
        const recommendedMimes = RECOMMENDED_MIMES[messageType];
        if (recommendedMimes && !recommendedMimes.some(mime => file.type.startsWith(mime.split('/')[0]))) {
            console.warn(`MIME type ${file.type} puede no ser compatible con ${messageType}`);
        }
        
        const preview = document.getElementById('filePreview');
        const fileName = document.getElementById('fileName');
        const fileSize = document.getElementById('fileSize');
        
        fileName.textContent = file.name;
        fileSize.textContent = `(${formatFileSize(file.size)})`;
        
        // Cambiar color según tamaño
        if (file.size > maxSize * 0.8) {
            fileSize.classList.add('text-yellow-600');
        } else {
            fileSize.classList.remove('text-yellow-600');
        }
        
        preview.classList.remove('hidden');
        
        // Log para debugging
        console.log(`Archivo seleccionado: ${file.name}, Tamaño: ${formatFileSize(file.size)}, MIME: ${file.type}`);
    }
}

// Handle file selection for auto
function handleFileSelectAuto(event) {
    const file = event.target.files[0];
    const messageType = document.getElementById('messageTypeAuto').value;
    
    if (file) {
        // Validar tamaño
        const maxSize = FILE_SIZE_LIMITS[messageType];
        if (file.size > maxSize) {
            showToast(`Archivo muy grande. Máximo: ${formatFileSize(maxSize)}`, 'error');
            event.target.value = ''; // Limpiar selección
            return;
        }
        
        // Validar MIME type (advertencia, no error)
        const recommendedMimes = RECOMMENDED_MIMES[messageType];
        if (recommendedMimes && !recommendedMimes.some(mime => file.type.startsWith(mime.split('/')[0]))) {
            console.warn(`MIME type ${file.type} puede no ser compatible con ${messageType}`);
        }
        
        const preview = document.getElementById('filePreviewAuto');
        const fileName = document.getElementById('fileNameAuto');
        const fileSize = document.getElementById('fileSizeAuto');
        
        fileName.textContent = file.name;
        fileSize.textContent = `(${formatFileSize(file.size)})`;
        
        // Cambiar color según tamaño
        if (file.size > maxSize * 0.8) {
            fileSize.classList.add('text-yellow-600');
        } else {
            fileSize.classList.remove('text-yellow-600');
        }
        
        preview.classList.remove('hidden');
        
        // Log para debugging
        console.log(`Archivo seleccionado: ${file.name}, Tamaño: ${formatFileSize(file.size)}, MIME: ${file.type}`);
    }
}

// Convert file to base64
function fileToBase64(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => {
            // Retornar el data URL completo (incluye data:image/png;base64,...)
            resolve(reader.result);
        };
        reader.onerror = reject;
        reader.readAsDataURL(file);
    });
}

// Format file size
function formatFileSize(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}
