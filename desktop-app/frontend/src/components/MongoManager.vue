<script lang="ts" setup>
import { ref, reactive, onMounted } from 'vue'
import {
  Connect, Disconnect, GetStatus, Up, Down, GetOplog, GetSchemaDiff, GetSchemaIndexes,
  GetDBHealth, GetOpslog, CreateMigration, StartMCPServer, StopMCPServer, GetMCPServerStatus,
  GetMCPActivity, SaveConnection, LoadConnections, DeleteConnection, ParseConnectionString,
  GetAIKey, SetAIKey, AskAI, GetMigrationsPath, SetMigrationsPath
} from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'

// Connection state
const connection = reactive({
  url: 'mongodb://localhost:27017',
  database: 'stackit',
  username: '',
  password: ''
})
const connected = ref(false)
const connectionMessage = ref('')
const connectionError = ref(false)

// Saved connections
const savedConnections = ref<main.SavedConnection[]>([])
const saveConnName = ref('')

// Migrations state
const migrations = ref<any[]>([])
const selectedTarget = ref('')
const dryRun = ref(false)
const migrationsPath = ref('')

// Oplog state
const oplogEntries = ref<any[]>([])
const oplogLimit = ref(10)

// Schema state
const schemaDiff = ref<any[]>([])
const schemaIndexes = ref<any[]>([])

// Health state
const healthReport = ref<any>(null)

// Opslog state
const opslogRecords = ref<any[]>([])
const opslogSearch = ref('')
const opslogVersion = ref('')
const opslogRegex = ref('')
const opslogFrom = ref('')
const opslogTo = ref('')
const opslogLimit = ref(100)

// Create migration state
const newMigrationName = ref('')

// MCP state
const mcpStatus = ref<any>(null)
const mcpTransport = ref('http')
const mcpListenAddr = ref('127.0.0.1:8080')
const mcpActivities = ref<any[]>([])
const mcpMessage = ref('')
const mcpError = ref(false)

// AI state
const aiKey = ref('')
const aiKeyInput = ref('')
const aiQuestion = ref('')
const aiAnswer = ref('')
const aiLoading = ref(false)
const aiError = ref(false)

// UI state
const activeTab = ref('connection')

// Feedback for actions
const migrationFeedback = reactive({ text: '', isError: false })

// Per-action loading flags
const loading = reactive({ up: false, down: false, createMigration: false, mcp: false })

// ── Connection actions ────────────────────────────────────────
async function connect() {
  connectionError.value = false
  connectionMessage.value = 'Connecting…'
  try {
    const result = await Connect(connection.url, connection.database, connection.username, connection.password)
    connected.value = true
    connectionMessage.value = result
    connectionError.value = false
    await loadStatus()
    // Auto-save connection if user has named it
    if (saveConnName.value.trim()) {
      await saveCurrentConnection()
    }
  } catch (err) {
    connectionMessage.value = String(err)
    connectionError.value = true
    connected.value = false
  }
}

async function disconnect() {
  try {
    const result = await Disconnect()
    connected.value = false
    connectionMessage.value = result
    connectionError.value = false
    migrations.value = []
  } catch (err) {
    connectionMessage.value = String(err)
    connectionError.value = true
  }
}

function parseURI() {
  ParseConnectionString(connection.url).then(parts => {
    if (parts.url) connection.url = parts.url
    if (parts.database) connection.database = parts.database
    if (parts.username) connection.username = parts.username
    if (parts.password) connection.password = parts.password
  }).catch(() => {})
}

async function loadSavedConnections() {
  try {
    savedConnections.value = await LoadConnections()
  } catch (err) {
    console.error('Failed to load connections:', err)
  }
}

function applyConnection(conn: main.SavedConnection) {
  connection.url = conn.url
  connection.database = conn.database
  connection.username = conn.username
  connection.password = conn.password
  saveConnName.value = conn.name
}

async function saveCurrentConnection() {
  if (!saveConnName.value.trim()) return
  const conn: main.SavedConnection = {
    name: saveConnName.value.trim(),
    url: connection.url,
    database: connection.database,
    username: connection.username,
    password: connection.password,
    last_used: new Date().toISOString()
  }
  await SaveConnection(conn)
  await loadSavedConnections()
}

async function deleteConnection(name: string) {
  await DeleteConnection(name)
  await loadSavedConnections()
}

// ── Migration actions ─────────────────────────────────────────
async function loadStatus() {
  if (!connected.value) return
  try {
    migrations.value = await GetStatus()
  } catch (err) {
    console.error('Failed to load status:', err)
  }
}

async function runUp() {
  if (!connected.value) return
  loading.up = true
  migrationFeedback.text = ''
  try {
    migrationFeedback.text = await Up(selectedTarget.value, dryRun.value)
    migrationFeedback.isError = false
    await loadStatus()
  } catch (err) {
    migrationFeedback.text = String(err)
    migrationFeedback.isError = true
  } finally {
    loading.up = false
  }
}

async function runDown() {
  if (!connected.value) return
  loading.down = true
  migrationFeedback.text = ''
  try {
    migrationFeedback.text = await Down(selectedTarget.value, dryRun.value)
    migrationFeedback.isError = false
    await loadStatus()
  } catch (err) {
    migrationFeedback.text = String(err)
    migrationFeedback.isError = true
  } finally {
    loading.down = false
  }
}

async function createMigration() {
  if (!newMigrationName.value.trim()) {
    migrationFeedback.text = 'Please enter a migration name'
    migrationFeedback.isError = true
    return
  }
  loading.createMigration = true
  migrationFeedback.text = ''
  try {
    migrationFeedback.text = await CreateMigration(newMigrationName.value)
    migrationFeedback.isError = false
    newMigrationName.value = ''
  } catch (err) {
    migrationFeedback.text = String(err)
    migrationFeedback.isError = true
  } finally {
    loading.createMigration = false
  }
}

async function loadMigrationsPath() {
  try {
    migrationsPath.value = await GetMigrationsPath()
  } catch {}
}

async function saveMigrationsPath() {
  try {
    await SetMigrationsPath(migrationsPath.value)
    migrationFeedback.text = 'Migrations path updated'
    migrationFeedback.isError = false
  } catch (err) {
    migrationFeedback.text = String(err)
    migrationFeedback.isError = true
  }
}

// ── Oplog actions ─────────────────────────────────────────────
async function loadOplog() {
  if (!connected.value) return
  try {
    oplogEntries.value = await GetOplog(oplogLimit.value)
  } catch (err) {
    console.error('Failed to load oplog:', err)
  }
}

// ── Schema actions ────────────────────────────────────────────
async function loadSchemaDiff() {
  if (!connected.value) return
  try {
    schemaDiff.value = await GetSchemaDiff()
  } catch (err) {
    console.error('Failed to load schema diff:', err)
  }
}

async function loadSchemaIndexes() {
  try {
    schemaIndexes.value = await GetSchemaIndexes()
  } catch (err) {
    console.error('Failed to load schema indexes:', err)
  }
}

// ── Health actions ────────────────────────────────────────────
async function loadHealth() {
  if (!connected.value) return
  try {
    healthReport.value = await GetDBHealth()
  } catch (err) {
    console.error('Failed to load health report:', err)
  }
}

// ── Opslog actions ────────────────────────────────────────────
async function loadOpslog() {
  if (!connected.value) return
  try {
    opslogRecords.value = await GetOpslog(
      opslogSearch.value, opslogVersion.value, opslogRegex.value,
      opslogFrom.value, opslogTo.value, opslogLimit.value
    )
  } catch (err) {
    console.error('Failed to load opslog:', err)
  }
}

// ── MCP actions ───────────────────────────────────────────────
async function refreshMCPStatus() {
  try {
    mcpStatus.value = await GetMCPServerStatus()
  } catch (err) {
    console.error('MCP status error:', err)
  }
}

async function startMCP() {
  loading.mcp = true
  mcpMessage.value = ''
  try {
    const result = await StartMCPServer(mcpTransport.value, mcpListenAddr.value)
    mcpMessage.value = result
    mcpError.value = false
    await refreshMCPStatus()
  } catch (err) {
    mcpMessage.value = String(err)
    mcpError.value = true
  } finally {
    loading.mcp = false
  }
}

async function stopMCP() {
  loading.mcp = true
  mcpMessage.value = ''
  try {
    const result = await StopMCPServer()
    mcpMessage.value = result
    mcpError.value = false
    await refreshMCPStatus()
  } catch (err) {
    mcpMessage.value = String(err)
    mcpError.value = true
  } finally {
    loading.mcp = false
  }
}

async function loadMCPActivity() {
  try {
    mcpActivities.value = await GetMCPActivity(20)
  } catch (err) {
    console.error('MCP activity error:', err)
  }
}

// ── AI actions ────────────────────────────────────────────────
async function loadAIKey() {
  try {
    aiKey.value = await GetAIKey()
    aiKeyInput.value = aiKey.value
  } catch {}
}

async function saveAIKey() {
  try {
    await SetAIKey(aiKeyInput.value)
    aiKey.value = aiKeyInput.value
  } catch (err) {
    console.error('Failed to save AI key:', err)
  }
}

async function askAI() {
  if (!aiQuestion.value.trim()) return
  aiLoading.value = true
  aiAnswer.value = ''
  aiError.value = false
  try {
    aiAnswer.value = await AskAI(aiQuestion.value)
  } catch (err) {
    aiAnswer.value = String(err)
    aiError.value = true
  } finally {
    aiLoading.value = false
  }
}

// ── Mount ─────────────────────────────────────────────────────
onMounted(async () => {
  await loadSavedConnections()
  await loadAIKey()
  await loadMigrationsPath()
  await refreshMCPStatus()
})
</script>

<template>
  <div class="app-container">
    <header class="header">
      <h1>MongoRK Desktop</h1>
      <div class="connection-status" :class="{ connected: connected }">
        {{ connected ? 'Connected' : 'Disconnected' }}
      </div>
    </header>

    <nav class="tabs">
      <button @click="activeTab = 'connection'" :class="{ active: activeTab === 'connection' }">Connection</button>
      <button @click="activeTab = 'migrations'" :class="{ active: activeTab === 'migrations' }">Migrations</button>
      <button @click="activeTab = 'opslog'" :class="{ active: activeTab === 'opslog' }">Opslog</button>
      <button @click="activeTab = 'oplog'" :class="{ active: activeTab === 'oplog' }">Oplog</button>
      <button @click="activeTab = 'schema'" :class="{ active: activeTab === 'schema' }">Schema Diff</button>
      <button @click="activeTab = 'health'" :class="{ active: activeTab === 'health' }">Health</button>
      <button @click="activeTab = 'mcp'; refreshMCPStatus()" :class="{ active: activeTab === 'mcp' }">MCP</button>
      <button @click="activeTab = 'ai'" :class="{ active: activeTab === 'ai' }">Mongork Advisor</button>
    </nav>

    <main class="content">
      <!-- Connection Tab -->
      <div v-if="activeTab === 'connection'" class="tab-content">
        <div class="form-group">
          <label>Connection URL</label>
          <div class="input-row">
            <input v-model="connection.url" type="text" placeholder="mongodb://localhost:27017" @paste.prevent="connection.url = $event.clipboardData?.getData('text') || connection.url; parseURI()" />
            <button class="btn-secondary" @click="parseURI" title="Parse URI fields">Parse</button>
          </div>
          <small class="hint">Paste a full URI and click Parse to auto-fill fields below</small>
        </div>
        <div class="form-group">
          <label>Database</label>
          <input v-model="connection.database" type="text" placeholder="stackit" />
        </div>
        <div class="form-group">
          <label>Username (optional)</label>
          <input v-model="connection.username" type="text" />
        </div>
        <div class="form-group">
          <label>Password (optional)</label>
          <input v-model="connection.password" type="password" />
        </div>
        <div class="button-group">
          <button @click="connect" :disabled="connected">Connect</button>
          <button @click="disconnect" :disabled="!connected" class="btn-danger">Disconnect</button>
        </div>
        <div v-if="connectionMessage" class="message" :class="{ 'feedback-error': connectionError, 'feedback-success': !connectionError && connectionMessage !== 'Connecting…' }">
          {{ connectionMessage }}
        </div>

        <!-- Saved connections -->
        <div class="saved-section">
          <h3>Save / Load Connection</h3>
          <div class="controls">
            <input v-model="saveConnName" placeholder="Connection name" />
            <button class="btn-secondary" @click="saveCurrentConnection" :disabled="!saveConnName.trim()">Save</button>
          </div>
          <div v-if="savedConnections.length" class="saved-list">
            <div v-for="conn in savedConnections" :key="conn.name" class="saved-item">
              <span class="saved-name">{{ conn.name }}</span>
              <span class="saved-url">{{ conn.url }} / {{ conn.database }}</span>
              <div class="saved-actions">
                <button class="btn-small" @click="applyConnection(conn)">Load</button>
                <button class="btn-small btn-danger-small" @click="deleteConnection(conn.name)">✕</button>
              </div>
            </div>
          </div>
          <div v-else class="empty-state small">No saved connections yet.</div>
        </div>
      </div>

      <!-- Migrations Tab -->
      <div v-if="activeTab === 'migrations'" class="tab-content">
        <div class="section-header">
          <div class="controls">
            <input v-model="selectedTarget" placeholder="Target version (optional)" />
            <label><input type="checkbox" v-model="dryRun" /> Dry Run</label>
            <button @click="runUp" :disabled="!connected || loading.up">{{ loading.up ? 'Running…' : 'Up' }}</button>
            <button @click="runDown" :disabled="!connected || loading.down">{{ loading.down ? 'Running…' : 'Down' }}</button>
            <button class="btn-secondary" @click="loadStatus" :disabled="!connected">Refresh</button>
          </div>
        </div>
        <div v-if="migrationFeedback.text" class="feedback-message" :class="{ 'feedback-error': migrationFeedback.isError, 'feedback-success': !migrationFeedback.isError }">
          {{ migrationFeedback.text }}
        </div>

        <div class="create-migration">
          <h3>Create New Migration</h3>
          <div class="controls">
            <input v-model="newMigrationName" placeholder="Migration name (e.g., add_users_table)" />
            <button @click="createMigration" :disabled="loading.createMigration">{{ loading.createMigration ? 'Creating…' : 'Create Migration' }}</button>
          </div>
          <div class="controls" style="margin-top:8px">
            <input v-model="migrationsPath" placeholder="Output path (e.g. /path/to/migrations)" style="flex:1" />
            <button class="btn-secondary" @click="saveMigrationsPath">Set Path</button>
          </div>
        </div>

        <table class="migrations-table" v-if="migrations.length">
          <thead>
            <tr>
              <th>Status</th>
              <th>Version</th>
              <th>Description</th>
              <th>Applied At</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="m in migrations" :key="m.version" :class="{ applied: m.applied }">
              <td>
                <span class="status-badge" :class="{ applied: m.applied }">
                  {{ m.applied ? '✓ Applied' : 'Pending' }}
                </span>
              </td>
              <td>{{ m.version }}</td>
              <td>{{ m.description }}</td>
              <td>{{ m.applied_at ? new Date(m.applied_at).toLocaleString() : '-' }}</td>
            </tr>
          </tbody>
        </table>
        <div v-else class="empty-state">No migrations found or not connected.</div>
      </div>

      <!-- Oplog Tab -->
      <div v-if="activeTab === 'oplog'" class="tab-content">
        <div class="controls">
          <label>Limit:</label>
          <input v-model.number="oplogLimit" type="number" min="1" max="100" style="width:80px" />
          <button @click="loadOplog" :disabled="!connected">Load Oplog</button>
        </div>
        <div class="oplog-entries">
          <div v-for="(entry, idx) in oplogEntries" :key="idx" class="oplog-entry">
            <pre>{{ JSON.stringify(entry, null, 2) }}</pre>
          </div>
        </div>
        <div v-if="!oplogEntries.length" class="empty-state">Click "Load Oplog" to fetch entries.</div>
      </div>

      <!-- Schema Diff Tab -->
      <div v-if="activeTab === 'schema'" class="tab-content">
        <div class="controls">
          <button @click="loadSchemaDiff" :disabled="!connected">Load Schema Diff</button>
          <button class="btn-secondary" @click="loadSchemaIndexes">Load Indexes</button>
        </div>
        <div v-if="schemaDiff.length" class="section">
          <h3>Schema Differences ({{ schemaDiff.length }})</h3>
          <table class="diff-table">
            <thead>
              <tr><th>Component</th><th>Action</th><th>Target</th><th>Current</th><th>Proposed</th><th>Risk</th></tr>
            </thead>
            <tbody>
              <tr v-for="(diff, idx) in schemaDiff" :key="idx">
                <td>{{ diff.component }}</td>
                <td><span class="badge" :class="diff.action">{{ diff.action }}</span></td>
                <td>{{ diff.target }}</td>
                <td><pre class="code">{{ diff.current }}</pre></td>
                <td><pre class="code">{{ diff.proposed }}</pre></td>
                <td>{{ diff.risk }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-if="schemaIndexes.length" class="section">
          <h3>Registered Indexes ({{ schemaIndexes.length }})</h3>
          <table class="index-table">
            <thead>
              <tr><th>Collection</th><th>Name</th><th>Keys</th><th>Unique</th><th>Sparse</th><th>Partial Filter</th><th>TTL</th></tr>
            </thead>
            <tbody>
              <tr v-for="(idx, i) in schemaIndexes" :key="i">
                <td>{{ idx.collection }}</td>
                <td>{{ idx.name }}</td>
                <td><code>{{ idx.keys }}</code></td>
                <td>{{ idx.unique ? '✓' : '' }}</td>
                <td>{{ idx.sparse ? '✓' : '' }}</td>
                <td><code>{{ idx.partial_filter || '-' }}</code></td>
                <td>{{ idx.expire_after_seconds ? idx.expire_after_seconds + 's' : '-' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-if="!schemaDiff.length && !schemaIndexes.length" class="empty-state">No schema data loaded.</div>
      </div>

      <!-- Opslog Tab -->
      <div v-if="activeTab === 'opslog'" class="tab-content">
        <div class="controls">
          <input v-model="opslogSearch" placeholder="Search text" />
          <input v-model="opslogVersion" placeholder="Version" />
          <input v-model="opslogRegex" placeholder="Regex" />
          <input v-model="opslogFrom" placeholder="From (YYYY-MM-DD)" />
          <input v-model="opslogTo" placeholder="To (YYYY-MM-DD)" />
          <input v-model.number="opslogLimit" type="number" placeholder="Limit" min="1" max="1000" style="width:80px" />
          <button @click="loadOpslog" :disabled="!connected">Load Opslog</button>
        </div>
        <table class="opslog-table" v-if="opslogRecords.length">
          <thead>
            <tr><th>Version</th><th>Description</th><th>Applied At</th><th>Checksum</th></tr>
          </thead>
          <tbody>
            <tr v-for="rec in opslogRecords" :key="rec.version">
              <td>{{ rec.version }}</td>
              <td>{{ rec.description }}</td>
              <td>{{ rec.applied_at ? new Date(rec.applied_at).toLocaleString() : '-' }}</td>
              <td><code>{{ rec.checksum }}</code></td>
            </tr>
          </tbody>
        </table>
        <div v-else class="empty-state">No opslog records found.</div>
      </div>

      <!-- Health Tab -->
      <div v-if="activeTab === 'health'" class="tab-content">
        <div class="controls">
          <button @click="loadHealth" :disabled="!connected">Refresh Health</button>
        </div>
        <div v-if="healthReport" class="health-report">
          <div class="health-section">
            <h3>Database Info</h3>
            <table class="health-table">
              <tbody>
                <tr><th>Database</th><td>{{ healthReport.database }}</td></tr>
                <tr><th>Role</th><td>{{ healthReport.role }}</td></tr>
                <tr><th>Oplog Window</th><td>{{ healthReport.oplog_window }}</td></tr>
                <tr><th>Oplog Size</th><td>{{ healthReport.oplog_size }}</td></tr>
                <tr><th>Connections</th><td>{{ healthReport.connections }}</td></tr>
              </tbody>
            </table>
          </div>
          <div v-if="healthReport.lag && Object.keys(healthReport.lag).length" class="health-section">
            <h3>Replication Lag</h3>
            <table class="lag-table">
              <thead><tr><th>Host</th><th>Lag</th></tr></thead>
              <tbody>
                <tr v-for="(lag, host) in healthReport.lag" :key="host">
                  <td>{{ host }}</td><td>{{ lag }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-if="healthReport.warnings && healthReport.warnings.length" class="health-section">
            <h3>Warnings</h3>
            <ul class="warnings">
              <li v-for="(warn, idx) in healthReport.warnings" :key="idx">{{ warn }}</li>
            </ul>
          </div>
        </div>
        <div v-else class="empty-state">Click "Refresh Health" to load report.</div>
      </div>

      <!-- MCP Tab -->
      <div v-if="activeTab === 'mcp'" class="tab-content">
        <div class="mcp-status-bar" :class="{ 'mcp-running': mcpStatus?.running }">
          <span class="mcp-dot"></span>
          <span>{{ mcpStatus?.running ? 'MCP Server Running' : 'MCP Server Stopped' }}</span>
          <span v-if="mcpStatus?.running" class="mcp-addr">{{ mcpStatus.transport }} · {{ mcpStatus.listenAddr }}</span>
        </div>

        <div class="controls" style="margin-top:16px">
          <div class="form-inline">
            <label>Transport</label>
            <select v-model="mcpTransport">
              <option value="http">HTTP</option>
              <option value="stdio">stdio</option>
            </select>
          </div>
          <div class="form-inline" v-if="mcpTransport === 'http'">
            <label>Listen</label>
            <input v-model="mcpListenAddr" placeholder="127.0.0.1:8080" style="width:160px" />
          </div>
          <button @click="startMCP" :disabled="!connected || loading.mcp || mcpStatus?.running">
            {{ loading.mcp ? '…' : 'Start' }}
          </button>
          <button class="btn-danger" @click="stopMCP" :disabled="loading.mcp || !mcpStatus?.running">Stop</button>
          <button class="btn-secondary" @click="loadMCPActivity">Refresh Activity</button>
        </div>

        <div v-if="mcpMessage" class="feedback-message" :class="{ 'feedback-error': mcpError, 'feedback-success': !mcpError }">
          {{ mcpMessage }}
        </div>
        <div v-if="!connected && !mcpStatus?.running" class="feedback-message feedback-error">
          Connect to MongoDB first before starting the MCP server.
        </div>

        <div v-if="mcpStatus?.running" class="mcp-info-box">
          <strong>Endpoint:</strong> http://{{ mcpListenAddr }}/mcp<br>
          <strong>Transport:</strong> {{ mcpTransport }}
        </div>

        <div class="section" v-if="mcpActivities.length">
          <h3>Recent Activity</h3>
          <table class="opslog-table">
            <thead>
              <tr><th>Time</th><th>Actor</th><th>Tool</th><th>Detail</th><th>Status</th></tr>
            </thead>
            <tbody>
              <tr v-for="(act, idx) in mcpActivities" :key="idx">
                <td>{{ new Date(act.timestamp).toLocaleTimeString() }}</td>
                <td>{{ act.actor }}</td>
                <td><code>{{ act.tool }}</code></td>
                <td>{{ act.detail }}</td>
                <td><span class="badge" :class="act.success ? 'create' : 'drop'">{{ act.success ? 'ok' : 'err' }}</span></td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="empty-state">No MCP activity recorded yet.</div>
      </div>

      <!-- AI Advisor Tab -->
      <div v-if="activeTab === 'ai'" class="tab-content">
        <div class="ai-key-section">
          <h3>Anthropic API Key</h3>
          <div class="controls">
            <input v-model="aiKeyInput" type="password" placeholder="sk-ant-…" style="flex:1" />
            <button class="btn-secondary" @click="saveAIKey">Save Key</button>
          </div>
          <small class="hint">Stored locally at ~/.config/mongork/settings.json</small>
        </div>

        <div class="ai-chat">
          <h3>Ask Mongork</h3>
          <textarea
            v-model="aiQuestion"
            placeholder="Ask about migrations, schema design, indexing strategies, database health…"
            rows="4"
            class="ai-input"
          ></textarea>
          <div class="controls" style="margin-top:8px">
            <button @click="askAI" :disabled="aiLoading || !aiKey">
              {{ aiLoading ? 'Thinking…' : 'Ask Mongork' }}
            </button>
            <span v-if="!aiKey" class="hint">Set your API key above first</span>
          </div>
          <div v-if="aiAnswer" class="ai-answer" :class="{ 'feedback-error': aiError }">
            <pre class="ai-text">{{ aiAnswer }}</pre>
          </div>
        </div>
      </div>
    </main>
  </div>
</template>

<style scoped>
/* ── Layout ─────────────────────────────────────────────────── */
.app-container {
  display: grid;
  grid-template-rows: 56px 1fr;
  grid-template-columns: 210px 1fr;
  grid-template-areas:
    "header header"
    "sidebar content";
  height: 100vh;
  background: #1c1b22;
  color: #e0dff0;
  font-size: 14px;
  overflow: hidden;
}

/* ── Header ─────────────────────────────────────────────────── */
.header {
  grid-area: header;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 24px;
  background: #211f2b;
  border-bottom: 1px solid #35324a;
  z-index: 10;
}
.header h1 {
  margin: 0;
  font-size: 17px;
  font-weight: 700;
  color: #fff;
  letter-spacing: -0.01em;
}
.connection-status {
  padding: 4px 12px;
  border-radius: 20px;
  background: #2e2c3e;
  color: #7a78a0;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.03em;
  text-transform: uppercase;
}
.connection-status.connected {
  background: #0d2e22;
  color: #1ea672;
  border: 1px solid rgba(30, 166, 114, 0.4);
}

/* ── Sidebar ─────────────────────────────────────────────────── */
.tabs {
  grid-area: sidebar;
  display: flex;
  flex-direction: column;
  background: #211f2b;
  border-right: 1px solid #35324a;
  padding: 12px 0;
  overflow-y: auto;
  gap: 2px;
}
.tabs button {
  display: flex;
  align-items: center;
  padding: 9px 20px;
  background: none;
  border: none;
  border-left: 3px solid transparent;
  cursor: pointer;
  font-size: 13px;
  font-weight: 500;
  color: #7a78a0;
  text-align: left;
  transition: background 0.12s, color 0.12s;
  white-space: nowrap;
}
.tabs button:hover {
  background: #2a2836;
  color: #c5c3db;
}
.tabs button.active {
  background: #2e2a4a;
  color: #8a70ff;
  border-left-color: #6d4aff;
  font-weight: 600;
}

/* ── Content area ─────────────────────────────────────────────── */
.content {
  grid-area: content;
  overflow-y: auto;
  background: #1c1b22;
}
.tab-content {
  padding: 28px 32px;
  max-width: 960px;
}

/* ── Typography ─────────────────────────────────────────────── */
h3 {
  margin: 0 0 14px 0;
  font-size: 13px;
  font-weight: 700;
  color: #a7a4bf;
  text-transform: uppercase;
  letter-spacing: 0.06em;
}

/* ── Form groups ─────────────────────────────────────────────── */
.form-group {
  margin-bottom: 18px;
}
.form-group label {
  display: block;
  margin-bottom: 6px;
  font-size: 13px;
  font-weight: 500;
  color: #a7a4bf;
}
.form-group input {
  width: 100%;
  padding: 9px 12px;
  background: #252433;
  border: 1px solid #3c3a52;
  border-radius: 6px;
  color: #e0dff0;
  font-size: 14px;
  box-sizing: border-box;
  transition: border-color 0.15s;
}
.form-group input:focus {
  outline: none;
  border-color: #6d4aff;
  box-shadow: 0 0 0 3px rgba(109, 74, 255, 0.15);
}
.input-row {
  display: flex;
  gap: 8px;
}
.input-row input { flex: 1; }

.hint {
  color: #6a6884;
  font-size: 12px;
  margin-top: 5px;
  display: block;
}

/* ── Buttons ─────────────────────────────────────────────────── */
.button-group {
  display: flex;
  gap: 8px;
  margin-bottom: 18px;
}
.button-group button,
.controls button {
  padding: 8px 18px;
  background: #6d4aff;
  color: #fff;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
  transition: background 0.15s, opacity 0.15s;
}
.button-group button:hover:not(:disabled),
.controls button:hover:not(:disabled) {
  background: #7c5cff;
}
.button-group button:disabled,
.controls button:disabled {
  background: #2e2c3e;
  color: #4e4c66;
  cursor: not-allowed;
}
.btn-secondary {
  background: #35324a !important;
  color: #c5c3db !important;
}
.btn-secondary:hover:not(:disabled) {
  background: #3e3b57 !important;
}
.btn-danger {
  background: #5c1a28 !important;
  color: #ff8095 !important;
  border: 1px solid rgba(220, 50, 81, 0.35) !important;
}
.btn-danger:hover:not(:disabled) {
  background: #7a2236 !important;
}
.btn-danger:disabled {
  background: #2e2c3e !important;
  color: #4e4c66 !important;
  border-color: transparent !important;
}
.btn-small {
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 600;
  background: #35324a;
  color: #c5c3db;
  border: none;
  border-radius: 5px;
  cursor: pointer;
  transition: background 0.12s;
}
.btn-small:hover { background: #3e3b57; }
.btn-danger-small {
  background: #5c1a28 !important;
  color: #ff8095 !important;
}
.btn-danger-small:hover { background: #7a2236 !important; }

/* ── Feedback messages ──────────────────────────────────────── */
.message {
  margin-top: 12px;
  padding: 10px 14px;
  border-radius: 6px;
  background: #1e1d2e;
  border: 1px solid #35324a;
}
.feedback-message {
  margin: 12px 0;
  padding: 10px 14px;
  border-radius: 6px;
  font-size: 13px;
}
.feedback-success {
  background: #0a2318;
  color: #1ea672;
  border-left: 3px solid #1ea672;
  border: 1px solid rgba(30, 166, 114, 0.25);
  border-left: 3px solid #1ea672;
}
.feedback-error {
  background: #200d12;
  color: #ff8095;
  border: 1px solid rgba(220, 50, 81, 0.25);
  border-left: 3px solid #dc3251;
}

/* ── Saved connections ──────────────────────────────────────── */
.saved-section {
  margin-top: 32px;
  padding-top: 24px;
  border-top: 1px solid #2e2c3e;
}
.saved-list { margin-top: 12px; }
.saved-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  border: 1px solid #2e2c3e;
  border-radius: 8px;
  margin-bottom: 6px;
  background: #211f2b;
  transition: border-color 0.12s;
}
.saved-item:hover { border-color: #3e3b57; }
.saved-name {
  font-weight: 600;
  color: #c5c3db;
  min-width: 120px;
}
.saved-url {
  flex: 1;
  font-size: 12px;
  color: #6a6884;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.saved-actions {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
}

/* ── Controls row ─────────────────────────────────────────────── */
.controls {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 20px;
  flex-wrap: wrap;
}
.controls input {
  padding: 8px 12px;
  background: #252433;
  border: 1px solid #3c3a52;
  border-radius: 6px;
  color: #e0dff0;
  font-size: 13px;
  transition: border-color 0.15s;
}
.controls input:focus {
  outline: none;
  border-color: #6d4aff;
}
.controls label {
  color: #a7a4bf;
  white-space: nowrap;
  font-size: 13px;
}
.form-inline {
  display: flex;
  align-items: center;
  gap: 6px;
}
.form-inline label {
  font-size: 13px;
  color: #7a78a0;
}
.form-inline select {
  padding: 8px 10px;
  background: #252433;
  border: 1px solid #3c3a52;
  border-radius: 6px;
  color: #e0dff0;
  font-size: 13px;
}
.form-inline select:focus { outline: none; border-color: #6d4aff; }

/* ── Migrations ─────────────────────────────────────────────── */
.create-migration {
  margin-top: 28px;
  padding-top: 24px;
  border-top: 1px solid #2e2c3e;
}
.migrations-table {
  width: 100%;
  border-collapse: collapse;
  margin-top: 20px;
}
.migrations-table th,
.migrations-table td {
  padding: 10px 14px;
  border: 1px solid #2e2c3e;
  text-align: left;
  font-size: 13px;
}
.migrations-table th {
  background: #211f2b;
  color: #7a78a0;
  font-weight: 600;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.migrations-table tr:hover td { background: #211f2b; }
.status-badge {
  padding: 3px 9px;
  border-radius: 20px;
  background: rgba(230, 92, 0, 0.2);
  color: #e6a84a;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.03em;
}
.status-badge.applied {
  background: rgba(30, 166, 114, 0.15);
  color: #1ea672;
}

/* ── MCP ─────────────────────────────────────────────────────── */
.mcp-status-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 18px;
  background: #200d12;
  border: 1px solid rgba(220, 50, 81, 0.25);
  border-radius: 8px;
  font-weight: 500;
  color: #ff8095;
}
.mcp-status-bar.mcp-running {
  background: #0a2318;
  border-color: rgba(30, 166, 114, 0.25);
  color: #1ea672;
}
.mcp-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #dc3251;
  flex-shrink: 0;
}
.mcp-running .mcp-dot {
  background: #1ea672;
  box-shadow: 0 0 6px rgba(30, 166, 114, 0.6);
}
.mcp-addr {
  font-size: 12px;
  color: #6a6884;
  margin-left: auto;
}
.mcp-info-box {
  background: #1a1928;
  border: 1px solid #2e2c3e;
  padding: 14px 18px;
  border-radius: 8px;
  font-size: 13px;
  line-height: 2;
  margin: 12px 0;
  color: #c5c3db;
}

/* ── Oplog ───────────────────────────────────────────────────── */
.oplog-entry {
  background: #211f2b;
  border: 1px solid #2e2c3e;
  padding: 12px 14px;
  margin-bottom: 8px;
  border-radius: 8px;
}
pre {
  margin: 0;
  white-space: pre-wrap;
  font-size: 12px;
  color: #c5c3db;
}
.code {
  font-family: 'SF Mono', 'Fira Code', Consolas, monospace;
  background: #1a1928;
  color: #a78bff;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 12px;
}

/* ── Badges ──────────────────────────────────────────────────── */
.badge {
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.badge.create { background: rgba(30, 166, 114, 0.2); color: #1ea672; }
.badge.drop   { background: rgba(220, 50, 81, 0.2);  color: #dc3251; }
.badge.alter  { background: rgba(230, 168, 74, 0.2); color: #e6a84a; }
.badge.add    { background: rgba(109, 74, 255, 0.2); color: #8a70ff; }
.badge.remove { background: rgba(167, 139, 255, 0.15); color: #c084fc; }

/* ── Tables ──────────────────────────────────────────────────── */
.section { margin-top: 32px; }
.section h3 {
  border-bottom: 1px solid #2e2c3e;
  padding-bottom: 10px;
}
.diff-table, .index-table, .opslog-table, .health-table, .lag-table {
  width: 100%;
  border-collapse: collapse;
  margin-top: 12px;
}
.diff-table th, .diff-table td,
.index-table th, .index-table td,
.opslog-table th, .opslog-table td,
.health-table th, .health-table td,
.lag-table th, .lag-table td {
  padding: 10px 14px;
  border: 1px solid #2e2c3e;
  text-align: left;
  vertical-align: top;
  font-size: 13px;
}
.diff-table th, .index-table th, .opslog-table th, .lag-table th {
  background: #211f2b;
  color: #7a78a0;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.diff-table tr:hover td,
.index-table tr:hover td,
.opslog-table tr:hover td,
.lag-table tr:hover td { background: #211f2b; }
.health-table th {
  background: #0a2318;
  color: #1ea672;
  width: 160px;
  font-size: 12px;
}
.health-table td {
  background: #1a1928;
  color: #c5c3db;
}

/* ── AI Advisor ─────────────────────────────────────────────── */
.ai-key-section {
  margin-bottom: 32px;
  padding-bottom: 24px;
  border-bottom: 1px solid #2e2c3e;
}
.ai-input {
  width: 100%;
  padding: 12px;
  background: #252433;
  border: 1px solid #3c3a52;
  border-radius: 8px;
  color: #e0dff0;
  font-size: 14px;
  font-family: inherit;
  resize: vertical;
  box-sizing: border-box;
  transition: border-color 0.15s;
}
.ai-input:focus {
  outline: none;
  border-color: #6d4aff;
  box-shadow: 0 0 0 3px rgba(109, 74, 255, 0.15);
}
.ai-answer {
  margin-top: 18px;
  padding: 18px;
  background: #1a1928;
  border-radius: 8px;
  border: 1px solid #2e2c3e;
  border-left: 3px solid #6d4aff;
}
.ai-answer.feedback-error {
  border-left-color: #dc3251;
  background: #200d12;
  border-color: rgba(220, 50, 81, 0.25);
}
.ai-text {
  white-space: pre-wrap;
  font-family: inherit;
  font-size: 14px;
  line-height: 1.7;
  margin: 0;
  color: #c5c3db;
}

/* ── Misc ─────────────────────────────────────────────────────── */
.empty-state {
  padding: 48px 32px;
  text-align: center;
  color: #4e4c66;
  font-size: 13px;
}
.empty-state.small {
  padding: 16px;
  font-size: 12px;
}
.health-report .health-section { margin-bottom: 32px; }
.warnings {
  color: #e6a84a;
  background: #1e1608;
  border: 1px solid rgba(230, 168, 74, 0.25);
  padding: 14px 18px;
  border-radius: 8px;
  list-style-position: inside;
  font-size: 13px;
  line-height: 1.8;
}

/* ── Checkbox accent ─────────────────────────────────────────── */
input[type="checkbox"] {
  accent-color: #6d4aff;
}
</style>