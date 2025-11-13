// Detectar la URL base automáticamente
const API_BASE = window.location.origin + '/api';

let currentLineForWebhook = null;
let refreshInterval = null;

// Load lines on page load
document.addEventListener('DOMContentLoaded', () => {
    loadLines();
    // Refresh every 5 seconds
    refreshInterval = setInterval(loadLines, 5000);
});

// Create new line
async function createLine() {
    const name = document.getElementById('lineName').value.trim();
    
    if (!name) {
        showToast('Por favor ingresa un nombre para la línea', 'error');
        return;
    }

    try {
        const response = await fetch(`${API_BASE}/lines`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ name }),
        });

        if (!response.ok) {
            throw new Error('Error al crear la línea');
        }

        const line = await response.json();
        document.getElementById('lineName').value = '';
        showToast('Línea creada exitosamente', 'success');
        
        // Wait a moment and then show QR if needed
        setTimeout(() => {
            loadLines();
            if (line.status === 'qr_pending') {
                showQRCode(line.id);
            }
        }, 1000);
    } catch (error) {
        console.error('Error:', error);
        showToast('Error al crear la línea', 'error');
    }
}

// Load all lines
async function loadLines() {
    try {
        const response = await fetch(`${API_BASE}/lines`);
        if (!response.ok) {
            throw new Error('Error al cargar las líneas');
        }

        const lines = await response.json();
        displayLines(lines || []);
    } catch (error) {
        console.error('Error:', error);
    }
}

// Display lines
function displayLines(lines) {
    const container = document.getElementById('linesList');
    
    if (!lines || lines.length === 0) {
        container.innerHTML = `
            <div class="text-center py-8 text-gray-500">
                <i class="fas fa-inbox text-4xl mb-2"></i>
                <p>No hay líneas registradas</p>
            </div>
        `;
        return;
    }

    container.innerHTML = lines.map(line => {
        const statusColors = {
            'connected': 'bg-green-100 text-green-800',
            'qr_pending': 'bg-yellow-100 text-yellow-800',
            'disconnected': 'bg-red-100 text-red-800'
        };

        const statusIcons = {
            'connected': 'fa-check-circle',
            'qr_pending': 'fa-qrcode',
            'disconnected': 'fa-times-circle'
        };

        const statusText = {
            'connected': 'Conectada',
            'qr_pending': 'Pendiente QR',
            'disconnected': 'Desconectada'
        };

        return `
            <div class="border border-gray-200 rounded-lg p-4 hover:shadow-md transition">
                <div class="flex items-center justify-between">
                    <div class="flex-1">
                        <div class="flex items-center space-x-3">
                            <h3 class="text-lg font-semibold text-gray-800">${line.name}</h3>
                            <span class="px-3 py-1 rounded-full text-xs font-medium ${statusColors[line.status]}">
                                <i class="fas ${statusIcons[line.status]} mr-1"></i>
                                ${statusText[line.status]}
                            </span>
                            ${line.available ? '<span class="px-3 py-1 rounded-full text-xs font-medium bg-blue-100 text-blue-800"><i class="fas fa-check mr-1"></i>Disponible</span>' : ''}
                            ${!line.active ? '<span class="px-3 py-1 rounded-full text-xs font-medium bg-gray-100 text-gray-800"><i class="fas fa-pause mr-1"></i>Pausada</span>' : ''}
                        </div>
                        <div class="mt-2 text-sm text-gray-600">
                            <p><strong>ID:</strong> ${line.id}</p>
                            ${line.webhook_url ? `<p class="flex items-center"><i class="fas fa-link mr-2"></i><strong>Webhook:</strong> <span class="ml-1 truncate max-w-xs">${line.webhook_url}</span></p>` : ''}
                            ${line.config && line.config.auto_reply_msg ? `<p class="flex items-center"><i class="fas fa-reply mr-2 text-purple-600"></i><strong>Auto-respuesta:</strong> Activa</p>` : ''}
                        </div>
                    </div>
                    <div class="flex space-x-2">
                        ${line.status === 'qr_pending' ? `
                            <button onclick="showQRCode('${line.id}')" class="px-4 py-2 bg-yellow-500 text-white rounded hover:bg-yellow-600 transition">
                                <i class="fas fa-qrcode mr-1"></i>Ver QR
                            </button>
                        ` : ''}
                        ${line.status === 'disconnected' || line.status === 'qr_pending' ? `
                            <button onclick="reconnectLine('${line.id}')" class="px-4 py-2 bg-indigo-500 text-white rounded hover:bg-indigo-600 transition">
                                <i class="fas fa-sync-alt mr-1"></i>Reconectar
                            </button>
                        ` : ''}
                        <button onclick="openConfigModal('${line.id}')" class="px-4 py-2 bg-purple-500 text-white rounded hover:bg-purple-600 transition">
                            <i class="fas fa-cog mr-1"></i>Configurar
                        </button>
                        <button onclick="openWebhookModal('${line.id}', '${line.webhook_url || ''}')" class="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600 transition">
                            <i class="fas fa-webhook mr-1"></i>Webhook
                        </button>
                        <button onclick="toggleLine('${line.id}', ${!line.active})" class="px-4 py-2 ${line.active ? 'bg-orange-500 hover:bg-orange-600' : 'bg-green-500 hover:bg-green-600'} text-white rounded transition">
                            <i class="fas fa-${line.active ? 'pause' : 'play'} mr-1"></i>${line.active ? 'Pausar' : 'Activar'}
                        </button>
                        <button onclick="deleteLine('${line.id}')" class="px-4 py-2 bg-red-500 text-white rounded hover:bg-red-600 transition">
                            <i class="fas fa-trash mr-1"></i>Eliminar
                        </button>
                    </div>
                </div>
            </div>
        `;
    }).join('');
}

// Show QR Code Modal
async function showQRCode(lineId) {
    const modal = document.getElementById('qrModal');
    const content = document.getElementById('qrContent');
    
    modal.classList.remove('hidden');
    content.innerHTML = `
        <div class="animate-pulse">
            <div class="bg-gray-200 h-64 w-64 mx-auto rounded"></div>
            <p class="mt-4 text-gray-600">Cargando código QR...</p>
        </div>
    `;

    try {
        const response = await fetch(`${API_BASE}/lines/${lineId}/qr`);
        if (!response.ok) {
            throw new Error('Error al obtener el código QR');
        }

        const data = await response.json();
        
        if (data.qr_code) {
            content.innerHTML = `
                <img src="${data.qr_code}" alt="QR Code" class="mx-auto rounded shadow-lg" />
                <p class="mt-4 text-gray-600">Escanea con WhatsApp</p>
            `;
            
            // Keep checking for connection
            const checkInterval = setInterval(async () => {
                const checkResponse = await fetch(`${API_BASE}/lines/${lineId}/qr`);
                const checkData = await checkResponse.json();
                
                if (checkData.status === 'connected') {
                    clearInterval(checkInterval);
                    showToast('¡Línea conectada exitosamente!', 'success');
                    closeQRModal();
                    loadLines();
                }
            }, 2000);
            
            // Stop checking after 2 minutes
            setTimeout(() => clearInterval(checkInterval), 120000);
        } else {
            content.innerHTML = `
                <p class="text-yellow-600">
                    <i class="fas fa-hourglass-half text-4xl mb-2"></i><br>
                    Esperando código QR...
                </p>
            `;
            
            // Retry after 2 seconds
            setTimeout(() => showQRCode(lineId), 2000);
        }
    } catch (error) {
        console.error('Error:', error);
        content.innerHTML = `
            <p class="text-red-600">
                <i class="fas fa-exclamation-circle text-4xl mb-2"></i><br>
                Error al cargar el código QR
            </p>
        `;
    }
}

// Close QR Modal
function closeQRModal() {
    document.getElementById('qrModal').classList.add('hidden');
}

// Open Webhook Modal
function openWebhookModal(lineId, currentUrl) {
    currentLineForWebhook = lineId;
    document.getElementById('webhookUrl').value = currentUrl;
    document.getElementById('webhookModal').classList.remove('hidden');
}

// Close Webhook Modal
function closeWebhookModal() {
    document.getElementById('webhookModal').classList.add('hidden');
    currentLineForWebhook = null;
}

// Save Webhook
async function saveWebhook() {
    const url = document.getElementById('webhookUrl').value.trim();
    
    if (!currentLineForWebhook) {
        return;
    }

    try {
        const response = await fetch(`${API_BASE}/lines/${currentLineForWebhook}/webhook`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ url }),
        });

        if (!response.ok) {
            throw new Error('Error al configurar el webhook');
        }

        showToast('Webhook configurado exitosamente', 'success');
        closeWebhookModal();
        loadLines();
    } catch (error) {
        console.error('Error:', error);
        showToast('Error al configurar el webhook', 'error');
    }
}

// Delete Line
async function deleteLine(lineId) {
    if (!confirm('¿Estás seguro de que deseas eliminar esta línea?')) {
        return;
    }

    try {
        const response = await fetch(`${API_BASE}/lines/${lineId}`, {
            method: 'DELETE',
        });

        if (!response.ok) {
            throw new Error('Error al eliminar la línea');
        }

        showToast('Línea eliminada exitosamente', 'success');
        loadLines();
    } catch (error) {
        console.error('Error:', error);
        showToast('Error al eliminar la línea', 'error');
    }
}

// Toggle Line Active/Inactive
async function toggleLine(lineId, active) {
    const action = active ? 'activar' : 'pausar';
    if (!confirm(`¿Estás seguro de que deseas ${action} esta línea?`)) {
        return;
    }

    try {
        const response = await fetch(`${API_BASE}/lines/${lineId}/toggle`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ active }),
        });

        if (!response.ok) {
            throw new Error(`Error al ${action} la línea`);
        }

        const result = await response.json();
        showToast(result.message, 'success');
        loadLines();
    } catch (error) {
        console.error('Error:', error);
        showToast(`Error al ${action} la línea`, 'error');
    }
}

// Reconnect Line
async function reconnectLine(lineId) {
    if (!confirm('¿Estás seguro de que deseas reconectar esta línea? Se generará un nuevo código QR.')) {
        return;
    }

    try {
        const response = await fetch(`${API_BASE}/lines/${lineId}/reconnect`, {
            method: 'POST',
        });

        if (!response.ok) {
            throw new Error('Error al reconectar la línea');
        }

        const result = await response.json();
        showToast(result.message, 'success');
        
        // Esperar un momento y mostrar el QR
        setTimeout(() => {
            loadLines();
            showQRCode(lineId);
        }, 1500);
    } catch (error) {
        console.error('Error:', error);
        showToast('Error al reconectar la línea', 'error');
    }
}

// Show Toast
function showToast(message, type = 'info') {
    const toast = document.getElementById('toast');
    const toastMessage = document.getElementById('toastMessage');
    
    if (!toast) {
        // Create toast for index.html
        const newToast = document.createElement('div');
        newToast.id = 'toast';
        newToast.className = 'fixed bottom-4 right-4 px-6 py-3 rounded-lg shadow-lg';
        newToast.innerHTML = `<p id="toastMessage"></p>`;
        document.body.appendChild(newToast);
        
        showToast(message, type);
        return;
    }

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

// Config Modal Management
let currentLineForConfig = null;

// Open Config Modal
async function openConfigModal(lineId) {
    currentLineForConfig = lineId;
    
    try {
        const response = await fetch(`${API_BASE}/lines/${lineId}`);
        if (!response.ok) {
            throw new Error('Error al obtener la línea');
        }

        const line = await response.json();
        const config = line.config || {
            allow_calls: false,
            respond_to_groups: false,
            auto_mark_read: true,
            always_online: true,
            auto_reply_msg: ''
        };

        // Set form values
        document.getElementById('configAllowCalls').checked = config.allow_calls;
        document.getElementById('configRespondToGroups').checked = config.respond_to_groups;
        document.getElementById('configAutoMarkRead').checked = config.auto_mark_read;
        document.getElementById('configAlwaysOnline').checked = config.always_online;
        document.getElementById('configAutoReplyMsg').value = config.auto_reply_msg || '';

        document.getElementById('configModal').classList.remove('hidden');
    } catch (error) {
        console.error('Error:', error);
        showToast('Error al cargar la configuración', 'error');
    }
}

// Close Config Modal
function closeConfigModal() {
    document.getElementById('configModal').classList.add('hidden');
    currentLineForConfig = null;
}

// Save Config
async function saveConfig() {
    if (!currentLineForConfig) {
        return;
    }

    const config = {
        allow_calls: document.getElementById('configAllowCalls').checked,
        respond_to_groups: document.getElementById('configRespondToGroups').checked,
        auto_mark_read: document.getElementById('configAutoMarkRead').checked,
        always_online: document.getElementById('configAlwaysOnline').checked,
        auto_reply_msg: document.getElementById('configAutoReplyMsg').value.trim()
    };

    try {
        const response = await fetch(`${API_BASE}/lines/${currentLineForConfig}/config`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify(config),
        });

        if (!response.ok) {
            throw new Error('Error al actualizar configuración');
        }

        showToast('Configuración actualizada exitosamente', 'success');
        closeConfigModal();
        loadLines();
    } catch (error) {
        console.error('Error:', error);
        showToast('Error al actualizar configuración', 'error');
    }
}
