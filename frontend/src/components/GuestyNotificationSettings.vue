<template>
  <v-card class="pa-6">
    <v-card-title>Cài đặt thông báo</v-card-title>

    <v-switch
      v-model="form.is_enabled"
      label="Bật thông báo"
      color="primary"
      class="mb-4"
    />

    <v-divider class="mb-4" />

    <!-- Telegram Section -->
    <div class="mb-6">
      <div class="d-flex align-center mb-3">
        <v-icon color="blue" class="mr-2">mdi-telegram</v-icon>
        <h3 class="text-h6">Telegram</h3>
        <v-switch v-model="form.telegram_enabled" class="ml-auto" color="blue" />
      </div>

      <template v-if="form.telegram_enabled">
        <v-text-field
          v-model="form.telegram_config.bot_token"
          label="Bot Token"
          hint="Nhập token từ @BotFather"
          persistent-hint
          type="password"
          variant="outlined"
          density="compact"
          class="mb-2"
        />
        <v-text-field
          v-model="form.telegram_config.chat_id"
          label="Chat ID"
          hint="ID nhóm Telegram nhận thông báo"
          persistent-hint
          variant="outlined"
          density="compact"
        />
      </template>
    </div>

    <v-divider class="mb-4" />

    <!-- Email Section -->
    <div class="mb-6">
      <div class="d-flex align-center mb-3">
        <v-icon color="orange" class="mr-2">mdi-email</v-icon>
        <h3 class="text-h6">Email</h3>
        <v-switch v-model="form.email_enabled" class="ml-auto" color="orange" />
      </div>

      <template v-if="form.email_enabled">
        <v-row class="mb-2">
          <v-col cols="12" sm="6">
            <v-text-field
              v-model="form.email_config.smtp_host"
              label="SMTP Host"
              hint="Ví dụ: smtp.gmail.com"
              persistent-hint
              variant="outlined"
              density="compact"
            />
          </v-col>
          <v-col cols="12" sm="6">
            <v-text-field
              v-model.number="form.email_config.smtp_port"
              label="SMTP Port"
              hint="Mặc định: 587"
              type="number"
              variant="outlined"
              density="compact"
            />
          </v-col>
        </v-row>
        <v-row class="mb-2">
          <v-col cols="12" sm="6">
            <v-text-field
              v-model="form.email_config.smtp_user"
              label="Username"
              variant="outlined"
              density="compact"
            />
          </v-col>
          <v-col cols="12" sm="6">
            <v-text-field
              v-model="form.email_config.smtp_pass"
              label="Password"
              type="password"
              variant="outlined"
              density="compact"
            />
          </v-col>
        </v-row>
        <v-text-field
          v-model="form.email_config.from"
          label="From Address"
          hint="Địa chỉ gửi (noreply@yourdomain.com)"
          persistent-hint
          variant="outlined"
          density="compact"
          class="mb-2"
        />
        <v-text-field
          v-model="form.email_config.to"
          label="To Addresses"
          hint="Địa chỉ nhận, cách nhau bằng dấu phẩy"
          persistent-hint
          variant="outlined"
          density="compact"
        />
      </template>
    </div>

    <v-divider class="mb-4" />

    <!-- Custom Template -->
    <div class="mb-4">
      <div class="d-flex align-center mb-3">
        <v-icon color="purple" class="mr-2">mdi-file-document-edit</v-icon>
        <h3 class="text-h6">Mẫu thông báo tùy chỉnh</h3>
        <v-switch v-model="form.use_custom_template" class="ml-auto" color="purple" />
      </div>

      <template v-if="form.use_custom_template">
        <v-alert type="info" variant="tonal" density="compact" class="mb-3">
          <div><strong>Biến sẵn có:</strong></div>
          <div class="mt-2">
            <code>&lbrace;&lbrace;category&rbrace;&rbrace;</code> - Loại vấn đề<br>
            <code>&lbrace;&lbrace;listing_name&rbrace;&rbrace;</code> - Tên căn hộ<br>
            <code>&lbrace;&lbrace;guest_name&rbrace;&rbrace;</code> - Tên khách<br>
            <code>&lbrace;&lbrace;reservation_id&rbrace;&rbrace;</code> - Mã đặt phòng<br>
            <code>&lbrace;&lbrace;summary&rbrace;&rbrace;</code> - Tóm tắt<br>
            <code>&lbrace;&lbrace;severity&rbrace;&rbrace;</code> - Mức độ nghiêm trọng<br>
            <code>&lbrace;&lbrace;confidence&rbrace;&rbrace;</code> - Độ tin cậy<br>
            <code>&lbrace;&lbrace;message&rbrace;&rbrace;</code> - Nội dung tin nhắn<br>
            <code>&lbrace;&lbrace;timestamp&rbrace;&rbrace;</code> - Thời gian
          </div>
        </v-alert>
        <v-textarea
          v-model="form.custom_template"
          label="Nội dung tùy chỉnh"
          rows="8"
          variant="outlined"
          auto-grow
          counter="10000"
        />
        <v-btn
          v-if="form.custom_template !== defaultTemplate"
          variant="text"
          size="x-small"
          color="primary"
          class="mt-2"
          @click="form.custom_template = defaultTemplate"
        >
          <v-icon start size="small">mdi-restore</v-icon>
          Khôi phục mặc định
        </v-btn>
      </template>
    </div>

    <!-- Save Error Alert -->
    <v-alert
      v-if="saveError"
      type="error"
      variant="tonal"
      density="compact"
      class="mb-4"
      closable
      @click:close="saveError = ''"
    >
      {{ saveError }}
    </v-alert>

    <v-card-actions class="mt-4">
      <v-spacer />
      <v-btn variant="text" @click="$emit('cancel')">Hủy</v-btn>
      <v-btn
        variant="tonal"
        @click="testSettings"
        :loading="testing"
        :disabled="!canTest"
      >
        <v-icon start>mdi-send-check</v-icon>
        Kiểm tra
      </v-btn>
      <!-- Test Result Chip -->
      <v-chip
        v-if="testResult"
        :color="testResult.success ? 'success' : 'error'"
        size="small"
        variant="tonal"
        class="ml-2"
      >
        <v-icon start size="small">{{ testResult.success ? 'mdi-check' : 'mdi-alert-circle' }}</v-icon>
        {{ testResult.message }}
      </v-chip>
      <v-btn
        color="primary"
        @click="saveSettings"
        :loading="saving"
      >
        Lưu cài đặt
      </v-btn>
    </v-card-actions>
  </v-card>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import api from '../api'

const props = defineProps<{
  tenantId: string
  channelId: string
}>()

const emit = defineEmits(['cancel', 'saved'])

const defaultTemplate = `🚨 URGENT: {{category}} issue detected

🏠 Property: {{listing_name}}
👤 Guest: {{guest_name}}
📅 Reservation: {{reservation_id}}

❌ Issue: {{summary}}
⚠️ Severity: {{severity}}
📊 Confidence: {{confidence}}

💬 Message:
{{message}}`

const form = reactive({
  is_enabled: true,
  telegram_enabled: false,
  telegram_config: {
    bot_token: '',
    chat_id: ''
  },
  email_enabled: false,
  email_config: {
    smtp_host: '',
    smtp_port: 587,
    smtp_user: '',
    smtp_pass: '',
    from: '',
    to: ''
  },
  use_custom_template: false,
  custom_template: ''
})

const saving = ref(false)
const testing = ref(false)
const saveError = ref('')
const testResult = ref<{ success: boolean; message: string } | null>(null)

const canTest = computed(() => {
  return (form.telegram_enabled && form.telegram_config.bot_token && form.telegram_config.chat_id) ||
         (form.email_enabled && form.email_config.smtp_host && form.email_config.to)
})

async function loadSettings() {
  try {
    const { data } = await api.get(`/tenants/${props.tenantId}/guesty/${props.channelId}/notifications`)
    Object.assign(form, data)
    // Set default template if empty
    if (form.use_custom_template && !form.custom_template) {
      form.custom_template = defaultTemplate
    }
  } catch (e: any) {
    console.error('Failed to load settings:', e)
  }
}

async function saveSettings() {
  saving.value = true
  saveError.value = ''
  try {
    await api.put(`/tenants/${props.tenantId}/guesty/${props.channelId}/notifications`, form)
    emit('saved')
  } catch (e: any) {
    const errorMsg = e.response?.data?.error || e.message
    saveError.value = 'Lưu thất bại: ' + errorMsg
  } finally {
    saving.value = false
  }
}

async function testSettings() {
  testing.value = true
  testResult.value = null
  saveError.value = ''
  try {
    await api.post(`/tenants/${props.tenantId}/guesty/${props.channelId}/notifications/test`)
    testResult.value = { success: true, message: 'Gửi thành công!' }
  } catch (e: any) {
    const msg = e.response?.data?.error || e.message
    testResult.value = { success: false, message: 'Gửi thất bại: ' + msg }
  } finally {
    testing.value = false
  }
}

// Load on mount
onMounted(() => {
  loadSettings()
})
</script>
