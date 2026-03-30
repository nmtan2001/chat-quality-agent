<template>
  <div>
    <h3 class="text-h6 mb-2">{{ $t('select_channels') }}</h3>
    <div class="text-body-2 text-grey-darken-1 mb-4">Chọn kênh chat cần phân tích. Hệ thống sẽ lấy cuộc hội thoại từ các kênh này.</div>
    <div v-if="!channels.length" class="text-center text-grey py-8">
      {{ $t('no_data') }}
    </div>
    <v-list v-else>
      <v-list-item v-for="ch in channels" :key="ch.id">
        <template #prepend>
          <v-checkbox-btn v-model="form.input_channel_ids" :value="ch.id" />
        </template>
        <v-list-item-title>{{ ch.name }}</v-list-item-title>
        <v-list-item-subtitle>
          <v-chip size="x-small" :color="getChannelColor(ch.channel_type)" variant="tonal" class="mr-1">
            {{ getChannelLabel(ch.channel_type) }}
          </v-chip>
          <v-chip size="x-small" :color="ch.is_active ? 'success' : 'grey'" variant="tonal">
            {{ ch.is_active ? $t('active') : $t('inactive') }}
          </v-chip>
        </v-list-item-subtitle>
      </v-list-item>
    </v-list>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute } from 'vue-router'
import { useChannelStore } from '../../stores/channels'

const form = defineModel<Record<string, any>>('form', { required: true })
const route = useRoute()
const channelStore = useChannelStore()
const { channels } = storeToRefs(channelStore)

function getChannelLabel(type: string): string {
  switch (type) {
    case 'zalo_oa':
      return 'Zalo OA'
    case 'facebook':
      return 'Facebook'
    case 'guesty':
      return 'Guesty'
    default:
      return type
  }
}

function getChannelColor(type: string): string {
  switch (type) {
    case 'zalo_oa':
      return 'blue'
    case 'facebook':
      return 'indigo'
    case 'guesty':
      return 'purple'
    default:
      return 'grey'
  }
}

onMounted(() => {
  const tenantId = route.params.tenantId as string
  channelStore.fetchChannels(tenantId)
})
</script>
