// Detectar la URL base automáticamente
const API_BASE = window.location.origin + '/api';

let charts = {};

// Load stats on page load
document.addEventListener('DOMContentLoaded', () => {
    loadLines();
    loadStats();
    
    // Refresh every 30 seconds
    setInterval(loadStats, 30000);
});

// Load lines for filter
async function loadLines() {
    try {
        const response = await fetch(`${API_BASE}/lines`);
        if (!response.ok) throw new Error('Error al cargar líneas');

        const lines = await response.json();
        const select = document.getElementById('lineFilter');
        
        if (lines && lines.length > 0) {
            lines.forEach(line => {
                const option = document.createElement('option');
                option.value = line.id;
                option.textContent = line.name;
                select.appendChild(option);
            });
        }
    } catch (error) {
        console.error('Error:', error);
    }
}

// Load all statistics
async function loadStats() {
    const period = document.getElementById('periodFilter').value;
    const lineId = document.getElementById('lineFilter').value;
    const messageType = document.getElementById('typeFilter').value;
    
    try {
        // Build query parameters
        const params = new URLSearchParams({
            period: period,
            ...(lineId && { line_id: lineId }),
            ...(messageType && { message_type: messageType })
        });

        const response = await fetch(`${API_BASE}/stats?${params}`);
        if (!response.ok) throw new Error('Error al cargar estadísticas');

        const stats = await response.json();
        
        // Update overview cards
        updateOverview(stats.overview);
        
        // Update charts
        updateMessagesPerDayChart(stats.messages_per_day);
        updateLinesUsageChart(stats.lines_usage);
        updateMessageTypesChart(stats.message_types);
        updateHourlyDistributionChart(stats.hourly_distribution);
        
        // Update top contacts
        updateTopContacts(stats.top_contacts);
        
        // Update recent activity
        updateRecentActivity(stats.recent_activity);
        
    } catch (error) {
        console.error('Error:', error);
        showToast('Error al cargar estadísticas', 'error');
    }
}

// Update overview cards
function updateOverview(overview) {
    if (!overview) return;
    
    document.getElementById('totalMessages').textContent = overview.total_messages || 0;
    document.getElementById('totalSent').textContent = overview.total_sent || 0;
    document.getElementById('totalReceived').textContent = overview.total_received || 0;
    document.getElementById('totalLines').textContent = overview.total_lines || 0;
}

// Update messages per day chart
function updateMessagesPerDayChart(data) {
    const ctx = document.getElementById('messagesPerDayChart');
    
    if (!data || data.length === 0) {
        ctx.parentElement.innerHTML = '<p class="text-gray-500 text-center py-8">No hay datos disponibles</p>';
        return;
    }

    // Destroy previous chart if exists
    if (charts.messagesPerDay) {
        charts.messagesPerDay.destroy();
    }

    const labels = data.map(d => {
        const date = new Date(d.date);
        return date.toLocaleDateString('es-MX', { day: '2-digit', month: 'short' });
    });
    
    const sentData = data.map(d => d.sent || 0);
    const receivedData = data.map(d => d.received || 0);

    charts.messagesPerDay = new Chart(ctx, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [
                {
                    label: 'Enviados',
                    data: sentData,
                    borderColor: 'rgb(34, 197, 94)',
                    backgroundColor: 'rgba(34, 197, 94, 0.1)',
                    tension: 0.4,
                    fill: true
                },
                {
                    label: 'Recibidos',
                    data: receivedData,
                    borderColor: 'rgb(168, 85, 247)',
                    backgroundColor: 'rgba(168, 85, 247, 0.1)',
                    tension: 0.4,
                    fill: true
                }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: true,
            plugins: {
                legend: {
                    position: 'bottom'
                },
                tooltip: {
                    mode: 'index',
                    intersect: false
                }
            },
            scales: {
                y: {
                    beginAtZero: true,
                    ticks: {
                        precision: 0
                    }
                }
            }
        }
    });
}

// Update lines usage chart
function updateLinesUsageChart(data) {
    const ctx = document.getElementById('linesUsageChart');
    
    if (!data || data.length === 0) {
        ctx.parentElement.innerHTML = '<p class="text-gray-500 text-center py-8">No hay datos disponibles</p>';
        return;
    }

    // Destroy previous chart if exists
    if (charts.linesUsage) {
        charts.linesUsage.destroy();
    }

    // Generate colors for each line
    const colors = [
        'rgb(34, 197, 94)',   // green
        'rgb(59, 130, 246)',  // blue
        'rgb(168, 85, 247)',  // purple
        'rgb(249, 115, 22)',  // orange
        'rgb(236, 72, 153)',  // pink
        'rgb(14, 165, 233)',  // sky
        'rgb(139, 92, 246)',  // violet
        'rgb(234, 179, 8)'    // yellow
    ];

    const labels = data.map(d => d.line_name);
    const values = data.map(d => d.count);
    const backgroundColors = colors.slice(0, data.length);

    charts.linesUsage = new Chart(ctx, {
        type: 'bar',
        data: {
            labels: labels,
            datasets: [{
                label: 'Mensajes Enviados',
                data: values,
                backgroundColor: backgroundColors,
                borderColor: backgroundColors,
                borderWidth: 1
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: true,
            plugins: {
                legend: {
                    display: false
                }
            },
            scales: {
                y: {
                    beginAtZero: true,
                    ticks: {
                        precision: 0
                    }
                }
            }
        }
    });
}

// Update message types chart
function updateMessageTypesChart(data) {
    const ctx = document.getElementById('messageTypesChart');
    
    if (!data || data.length === 0) {
        ctx.parentElement.innerHTML = '<p class="text-gray-500 text-center py-8">No hay datos disponibles</p>';
        return;
    }

    // Destroy previous chart if exists
    if (charts.messageTypes) {
        charts.messageTypes.destroy();
    }

    const typeLabels = {
        'text': 'Texto',
        'image': 'Imagen',
        'audio': 'Audio',
        'video': 'Video',
        'document': 'Documento',
        'voice': 'Nota de Voz'
    };

    const typeColors = {
        'text': 'rgb(59, 130, 246)',
        'image': 'rgb(34, 197, 94)',
        'audio': 'rgb(168, 85, 247)',
        'video': 'rgb(249, 115, 22)',
        'document': 'rgb(236, 72, 153)',
        'voice': 'rgb(234, 179, 8)'
    };

    const labels = data.map(d => typeLabels[d.type] || d.type);
    const values = data.map(d => d.count);
    const colors = data.map(d => typeColors[d.type] || 'rgb(156, 163, 175)');

    charts.messageTypes = new Chart(ctx, {
        type: 'doughnut',
        data: {
            labels: labels,
            datasets: [{
                data: values,
                backgroundColor: colors,
                borderWidth: 2,
                borderColor: '#fff'
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: true,
            plugins: {
                legend: {
                    position: 'bottom'
                }
            }
        }
    });
}

// Update hourly distribution chart
function updateHourlyDistributionChart(data) {
    const ctx = document.getElementById('hourlyDistributionChart');
    
    if (!data || data.length === 0) {
        ctx.parentElement.innerHTML = '<p class="text-gray-500 text-center py-8">No hay datos disponibles</p>';
        return;
    }

    // Destroy previous chart if exists
    if (charts.hourlyDistribution) {
        charts.hourlyDistribution.destroy();
    }

    // Create array for all 24 hours
    const hourlyData = new Array(24).fill(0);
    data.forEach(d => {
        if (d.hour >= 0 && d.hour < 24) {
            hourlyData[d.hour] = d.count;
        }
    });

    const labels = Array.from({length: 24}, (_, i) => `${i}:00`);

    charts.hourlyDistribution = new Chart(ctx, {
        type: 'bar',
        data: {
            labels: labels,
            datasets: [{
                label: 'Mensajes',
                data: hourlyData,
                backgroundColor: 'rgba(249, 115, 22, 0.8)',
                borderColor: 'rgb(249, 115, 22)',
                borderWidth: 1
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: true,
            plugins: {
                legend: {
                    display: false
                }
            },
            scales: {
                y: {
                    beginAtZero: true,
                    ticks: {
                        precision: 0
                    }
                }
            }
        }
    });
}

// Update top contacts
function updateTopContacts(data) {
    const container = document.getElementById('topContacts');
    
    if (!data || data.length === 0) {
        container.innerHTML = '<p class="text-gray-500 text-center py-8">No hay datos disponibles</p>';
        return;
    }

    container.innerHTML = data.map((contact, index) => {
        const rankColors = ['bg-yellow-100 text-yellow-800', 'bg-gray-200 text-gray-800', 'bg-orange-100 text-orange-800'];
        const rankColor = rankColors[index] || 'bg-blue-100 text-blue-800';
        
        return `
            <div class="flex items-center justify-between p-4 border border-gray-200 rounded-lg hover:bg-gray-50 transition">
                <div class="flex items-center space-x-4 flex-1">
                    <span class="w-8 h-8 flex items-center justify-center rounded-full ${rankColor} font-bold">
                        ${index + 1}
                    </span>
                    <div class="flex-1">
                        <p class="font-semibold text-gray-800">${contact.contact || 'Desconocido'}</p>
                        <p class="text-sm text-gray-600">
                            ${contact.sent || 0} enviados · ${contact.received || 0} recibidos
                        </p>
                    </div>
                </div>
                <div class="text-right">
                    <p class="text-2xl font-bold text-gray-800">${contact.total || 0}</p>
                    <p class="text-xs text-gray-500">mensajes</p>
                </div>
            </div>
        `;
    }).join('');
}

// Update recent activity
function updateRecentActivity(data) {
    const container = document.getElementById('recentActivity');
    
    if (!data || data.length === 0) {
        container.innerHTML = '<p class="text-gray-500 text-center py-8">No hay actividad reciente</p>';
        return;
    }

    const typeIcons = {
        'text': 'fa-comment',
        'image': 'fa-image',
        'audio': 'fa-music',
        'video': 'fa-video',
        'document': 'fa-file',
        'voice': 'fa-microphone'
    };

    container.innerHTML = data.map(activity => {
        const date = new Date(activity.timestamp);
        const timeStr = date.toLocaleString('es-MX', {
            day: '2-digit',
            month: 'short',
            hour: '2-digit',
            minute: '2-digit'
        });
        
        const directionColor = activity.direction === 'sent' ? 'text-green-600' : 'text-purple-600';
        const directionIcon = activity.direction === 'sent' ? 'fa-arrow-up' : 'fa-arrow-down';
        const directionText = activity.direction === 'sent' ? 'Enviado' : 'Recibido';
        const icon = typeIcons[activity.message_type] || 'fa-comment';
        
        return `
            <div class="flex items-start space-x-4 p-4 border border-gray-200 rounded-lg hover:bg-gray-50 transition">
                <div class="flex-shrink-0">
                    <div class="w-10 h-10 rounded-full bg-gray-100 flex items-center justify-center">
                        <i class="fas ${icon} text-gray-600"></i>
                    </div>
                </div>
                <div class="flex-1 min-w-0">
                    <div class="flex items-center space-x-2 mb-1">
                        <span class="px-2 py-1 rounded-full text-xs font-medium ${directionColor === 'text-green-600' ? 'bg-green-100 text-green-800' : 'bg-purple-100 text-purple-800'}">
                            <i class="fas ${directionIcon} mr-1"></i>${directionText}
                        </span>
                        <span class="text-xs text-gray-500">${activity.line_name || 'Línea'}</span>
                    </div>
                    <p class="text-sm font-medium text-gray-800 truncate">
                        ${activity.direction === 'sent' ? 'A' : 'De'}: ${activity.contact || 'Desconocido'}
                    </p>
                    ${activity.message_text ? `<p class="text-sm text-gray-600 truncate">${activity.message_text}</p>` : ''}
                    <p class="text-xs text-gray-500 mt-1">
                        <i class="fas fa-clock mr-1"></i>${timeStr}
                    </p>
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
