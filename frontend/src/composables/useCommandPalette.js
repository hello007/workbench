import { ref, computed } from 'vue'
import { SearchFiles } from '../../wailsjs/go/main/App'

export function useCommandPalette() {
  const visible = ref(false)
  const input = ref('')
  const selectedIndex = ref(0)
  const fileResults = ref([])
  const searchLoading = ref(false)

  const mode = computed(() => {
    if (input.value.startsWith('#')) return 'workdir'
    if (input.value.startsWith('@')) return 'favorites'
    if (input.value.startsWith('>')) return 'command'
    return 'general'
  })

  const query = computed(() => {
    if (mode.value !== 'general') {
      return input.value.slice(1).trim()
    }
    return input.value.trim()
  })

  function open() {
    visible.value = true
    input.value = ''
    selectedIndex.value = 0
    fileResults.value = []
  }

  function close() {
    visible.value = false
    input.value = ''
    fileResults.value = []
  }

  async function searchFiles(rootDir) {
    if (!query.value || mode.value !== 'general') {
      fileResults.value = []
      return
    }
    searchLoading.value = true
    try {
      fileResults.value = await SearchFiles(rootDir, query.value, 20)
    } catch (e) {
      fileResults.value = []
    } finally {
      searchLoading.value = false
    }
  }

  function moveSelection(delta) {
    const maxIndex = fileResults.value.length - 1
    selectedIndex.value = Math.max(0, Math.min(maxIndex, selectedIndex.value + delta))
  }

  function resetSelection() {
    selectedIndex.value = 0
  }

  return {
    visible,
    input,
    mode,
    query,
    selectedIndex,
    fileResults,
    searchLoading,
    open,
    close,
    searchFiles,
    moveSelection,
    resetSelection
  }
}
