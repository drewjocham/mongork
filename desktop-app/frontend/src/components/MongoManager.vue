<script lang="ts" setup>
import { ref, reactive, onMounted } from 'vue'
import { Connect, Disconnect, GetStatus, Up, Down, GetOplog, GetSchemaDiff, GetSchemaIndexes, GetDBHealth, GetOpslog, CreateMigration, StartMCPServer, StopMCPServer, GetMCPServerStatus, GetMCPActivity } from '../../wailsjs/go/main/App'
import { desktop } from '../../wailsjs/go/models'

// Connection state
const connection = reactive({
  url: 'mongodb://localhost:27017',
  database: 'test',
  username: '',
  password: ''
})
const connected = ref(false)
const connectionMessage = ref('')

// Migrations state
const migrations = ref<desktop.MigrationStatus[]>([])
const selectedTarget = ref('')
const dryRun = ref(false)

// Oplog state
const oplogEntries = ref<any[]>([])
const oplogLimit = ref(10)

// Schema state
const schemaDiff = ref<desktop.Diff[]>([])
const schemaIndexes = ref<desktop.IndexSpec[]>([])

// Health state
const healthReport = ref<desktop.HealthReport | null>(null)

// Opslog state
const opslogRecords = ref<desktop.MigrationRecord[]>([])
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
const mcpTransport = ref('stdio')
const mcpListenAddr = ref('0.0.0.0:8080')
const mcpActivities = ref<any[]>([])
const mcpActivityLimit = ref(20)

// UI state
const activeTab = ref('connection')

// Feedback for migration actions (replaces alert())
const migrationFeedback = reactive({ text: '', isError: false })

// Per-action loading flags to prevent double-clicks
const loading = reactive({ up: false, down: false, createMigration: false })

// Connection actions
async function connect() {
  try {
    const result = await Connect(connection.url, connection.database, connection.username, connection.password)
    connected.value = true
    connectionMessage.value = result
    await loadStatus()
  } catch (err) {
    connectionMessage.value = 'Error: ' + err
  }
}

async function disconnect() {
  try {
    const result = await Disconnect()
    connected.value = false
    connectionMessage.value = result
    migrations.value = []
  } catch (err) {
    connectionMessage.value = 'Error: ' + err
  }
}

// Migration actions
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
    migrationFeedback.text = 'Error: ' + err
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
    migrationFeedback.text = 'Error: ' + err
    migrationFeedback.isError = true
  } finally {
    loading.down = false
  }
}

// Oplog actions
async function loadOplog() {
  if (!connected.value) return
  try {
    oplogEntries.value = await GetOplog(oplogLimit.value)
  } catch (err) {
    console.error('Failed to load oplog:', err)
  }
}

// Schema actions
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

// Health actions
async function loadHealth() {
  if (!connected.value) return
  try {
    healthReport.value = await GetDBHealth()
  } catch (err) {
    console.error('Failed to load health report:', err)
  }
}

// Opslog actions
async function loadOpslog() {
  if (!connected.value) return
  try {
    opslogRecords.value = await GetOpslog(
      opslogSearch.value,
      opslogVersion.value,
      opslogRegex.value,
      opslogFrom.value,
      opslogTo.value,
      opslogLimit.value
    )
  } catch (err) {
    console.error('Failed to load opslog:', err)
  }
}

// Create migration
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
    migrationFeedback.text = 'Error: ' + err
    migrationFeedback.isError = true
  } finally {
    loading.createMigration = false
  }
}

// Initial load
onMounted(() => {
  // nothing for now
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
      <button @click="activeTab = 'mcp'" :class="{ active: activeTab === 'mcp' }">MCP</button>
    </nav>

    <main class="content">
      <!-- Connection Tab -->
      <div v-if="activeTab === 'connection'" class="tab-content">
        <div class="form-group">
          <label>Connection URL</label>
          <input v-model="connection.url" type="text" placeholder="mongodb://localhost:27017" />
        </div>
        <div class="form-group">
          <label>Database</label>
          <input v-model="connection.database" type="text" placeholder="test" />
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
          <button @click="disconnect" :disabled="!connected">Disconnect</button>
        </div>
        <div v-if="connectionMessage" class="message">{{ connectionMessage }}</div>
      </div>

      <!-- Migrations Tab -->
      <div v-if="activeTab === 'migrations'" class="tab-content">
        <div class="controls">
          <input v-model="selectedTarget" placeholder="Target version (optional)" />
          <label><input type="checkbox" v-model="dryRun" /> Dry Run</label>
          <button @click="runUp" :disabled="!connected || loading.up">{{ loading.up ? 'Running…' : 'Up' }}</button>
          <button @click="runDown" :disabled="!connected || loading.down">{{ loading.down ? 'Running…' : 'Down' }}</button>
          <button @click="loadStatus" :disabled="!connected">Refresh</button>
        </div>
        <div v-if="migrationFeedback.text" class="feedback-message" :class="{ 'feedback-error': migrationFeedback.isError, 'feedback-success': !migrationFeedback.isError }">
          {{ migrationFeedback.text }}
        </div>
        <div class="create-migration">
          <h3>Create New Migration</h3>
          <div class="controls">
            <input v-model="newMigrationName" placeholder="Migration name (e.g., add_users_table)" />
            <button @click="createMigration" :disabled="!connected || loading.createMigration">{{ loading.createMigration ? 'Creating…' : 'Create Migration' }}</button>
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
        <div v-else class="empty-state">
          No migrations found or not connected.
        </div>
      </div>

      <!-- Oplog Tab -->
      <div v-if="activeTab === 'oplog'" class="tab-content">
        <div class="controls">
          <label>Limit:</label>
          <input v-model.number="oplogLimit" type="number" min="1" max="100" />
          <button @click="loadOplog" :disabled="!connected">Load Oplog</button>
        </div>
        <div class="oplog-entries">
          <div v-for="(entry, idx) in oplogEntries" :key="idx" class="oplog-entry">
            <pre>{{ JSON.stringify(entry, null, 2) }}</pre>
          </div>
        </div>
      </div>

      <!-- Schema Diff Tab -->
      <div v-if="activeTab === 'schema'" class="tab-content">
        <div class="controls">
          <button @click="loadSchemaDiff" :disabled="!connected">Load Schema Diff</button>
          <button @click="loadSchemaIndexes">Load Indexes</button>
        </div>
        <div v-if="schemaDiff.length" class="section">
          <h3>Schema Differences ({{ schemaDiff.length }})</h3>
          <table class="diff-table">
            <thead>
              <tr>
                <th>Component</th>
                <th>Action</th>
                <th>Target</th>
                <th>Current</th>
                <th>Proposed</th>
                <th>Risk</th>
              </tr>
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
              <tr>
                <th>Collection</th>
                <th>Name</th>
                <th>Keys</th>
                <th>Unique</th>
                <th>Sparse</th>
                <th>Partial Filter</th>
                <th>TTL</th>
              </tr>
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
        <div v-if="!schemaDiff.length && !schemaIndexes.length" class="empty-state">
          No schema data loaded. Click buttons above to load.
        </div>
      </div>

      <!-- Opslog Tab -->
      <div v-if="activeTab === 'opslog'" class="tab-content">
        <div class="controls">
          <input v-model="opslogSearch" placeholder="Search text" />
          <input v-model="opslogVersion" placeholder="Version" />
          <input v-model="opslogRegex" placeholder="Regex" />
          <input v-model="opslogFrom" placeholder="From date (YYYY-MM-DD)" />
          <input v-model="opslogTo" placeholder="To date (YYYY-MM-DD)" />
          <input v-model.number="opslogLimit" type="number" placeholder="Limit" min="1" max="1000" />
          <button @click="loadOpslog" :disabled="!connected">Load Opslog</button>
        </div>
        <table class="opslog-table" v-if="opslogRecords.length">
          <thead>
            <tr>
              <th>Version</th>
              <th>Description</th>
              <th>Applied At</th>
              <th>Checksum</th>
            </tr>
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
        <div v-else class="empty-state">
          No opslog records found.
        </div>
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
              <tr><th>Database</th><td>{{ healthReport.database }}</td></tr>
              <tr><th>Role</th><td>{{ healthReport.role }}</td></tr>
              <tr><th>Oplog Window</th><td>{{ healthReport.oplog_window }}</td></tr>
              <tr><th>Oplog Size</th><td>{{ healthReport.oplog_size }}</td></tr>
              <tr><th>Connections</th><td>{{ healthReport.connections }}</td></tr>
            </table>
          </div>
          <div v-if="healthReport.lag && Object.keys(healthReport.lag).length" class="health-section">
            <h3>Replication Lag</h3>
            <table class="lag-table">
              <thead>
                <tr><th>Host</th><th>Lag</th></tr>
              </thead>
              <tbody>
                <tr v-for="(lag, host) in healthReport.lag" :key="host">
                  <td>{{ host }}</td>
                  <td>{{ lag }}</td>
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
          <div v-if="!healthReport" class="empty-state">
            Health report not loaded.
          </div>
        </div>
      </div>

      <!-- MCP Tab -->
      <div v-if="activeTab === 'mcp'" class="tab-content">
        <p>MCP server control coming soon.</p>
      </div>
    </main>
  </div>
</template>

<style scoped>
.app-container {
  padding: 20px;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
  background: white;
  color: #333;
  text-align: left;
  min-height: 100vh;
  box-sizing: border-box;
}
.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}
.connection-status {
  padding: 5px 10px;
  border-radius: 4px;
  background: #ccc;
  color: #333;
}
.connection-status.connected {
  background: #4caf50;
  color: white;
}
.tabs {
  display: flex;
  border-bottom: 1px solid #ddd;
  margin-bottom: 20px;
}
.tabs button {
  padding: 10px 20px;
  background: none;
  border: none;
  cursor: pointer;
  font-size: 16px;
  color: #555;
}
.tabs button.active {
  border-bottom: 2px solid #2196f3;
  font-weight: bold;
}
.tab-content {
  padding: 20px;
}
.form-group {
  margin-bottom: 15px;
}
.form-group label {
  display: block;
  margin-bottom: 5px;
  font-weight: 500;
}
.form-group input {
  width: 100%;
  padding: 8px;
  border: 1px solid #ccc;
  border-radius: 4px;
}
.button-group button {
  margin-right: 10px;
  padding: 10px 20px;
  background: #2196f3;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
}
.button-group button:disabled {
  background: #ccc;
  cursor: not-allowed;
}
.message {
  margin-top: 15px;
  padding: 10px;
  border-radius: 4px;
  background: #e3f2fd;
}
.migrations-table {
  width: 100%;
  border-collapse: collapse;
  margin-top: 20px;
}
.migrations-table th, .migrations-table td {
  padding: 10px;
  border: 1px solid #ddd;
  text-align: left;
}
.migrations-table th {
  background: #f5f5f5;
}
.status-badge {
  padding: 3px 8px;
  border-radius: 12px;
  background: #ff9800;
  color: white;
  font-size: 12px;
}
.status-badge.applied {
  background: #4caf50;
}
.empty-state {
  padding: 40px;
  text-align: center;
  color: #888;
}
.controls {
  display: flex;
  gap: 10px;
  align-items: center;
  margin-bottom: 20px;
  flex-wrap: wrap;
}
.controls input {
  padding: 8px;
  border: 1px solid #ccc;
  border-radius: 4px;
  color: #333;
  background: white;
  font-size: 14px;
}
.controls button {
  padding: 8px 16px;
  background: #2196f3;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-size: 14px;
  white-space: nowrap;
}
.controls button:disabled {
  background: #ccc;
  color: #666;
  cursor: not-allowed;
}
.controls label {
  color: #333;
  white-space: nowrap;
}
.oplog-entry {
  background: #f9f9f9;
  border: 1px solid #eee;
  padding: 10px;
  margin-bottom: 10px;
  border-radius: 4px;
}
pre {
  margin: 0;
  white-space: pre-wrap;
  font-size: 12px;
}
.code {
  font-family: monospace;
  background: #f5f5f5;
  padding: 2px 4px;
  border-radius: 3px;
  font-size: 12px;
}
.badge {
  padding: 3px 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: bold;
  text-transform: uppercase;
}
.badge.create { background: #4caf50; color: white; }
.badge.drop { background: #f44336; color: white; }
.badge.alter { background: #ff9800; color: white; }
.badge.add { background: #2196f3; color: white; }
.badge.remove { background: #9c27b0; color: white; }

.section {
  margin-top: 30px;
}
.section h3 {
  margin-bottom: 10px;
  color: #333;
  border-bottom: 1px solid #eee;
  padding-bottom: 5px;
}

.diff-table, .index-table, .opslog-table, .health-table, .lag-table {
  width: 100%;
  border-collapse: collapse;
  margin-top: 10px;
}
.diff-table th, .diff-table td,
.index-table th, .index-table td,
.opslog-table th, .opslog-table td,
.health-table th, .health-table td,
.lag-table th, .lag-table td {
  padding: 10px;
  border: 1px solid #ddd;
  text-align: left;
  vertical-align: top;
}
.diff-table th, .index-table th, .opslog-table th, .lag-table th {
  background: #f5f5f5;
}
.health-table th {
  background: #e8f5e9;
  width: 150px;
}
.health-table td {
  background: #f9f9f9;
}

.feedback-message {
  margin: 12px 0;
  padding: 10px 14px;
  border-radius: 4px;
  font-size: 14px;
}
.feedback-success {
  background: #e8f5e9;
  color: #2e7d32;
  border-left: 3px solid #4caf50;
}
.feedback-error {
  background: #ffebee;
  color: #c62828;
  border-left: 3px solid #f44336;
}
.create-migration {
  margin-top: 30px;
  padding-top: 20px;
  border-top: 1px solid #eee;
}
.create-migration h3 {
  margin-bottom: 10px;
}

.health-report .health-section {
  margin-bottom: 30px;
}
.warnings {
  color: #d32f2f;
  background: #ffebee;
  padding: 15px;
  border-radius: 4px;
  list-style-position: inside;
}
</style>