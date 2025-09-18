<template>
  <div ref="wrapperRef" class="relative inline-block w-full">
    <div class="relative">
      <input
        ref="inputRef"
        type="text"
        :placeholder="placeholder"
        :disabled="disabled"
        class="border border-gray-300 rounded px-2 py-1 pr-7 w-full text-sm focus:border-blue-500 focus:ring-1 focus:ring-blue-500 outline-none"
        :value="displayValue"
        @focus="openDropdown"
        @click="openDropdown"
        @input="onInput"
        @keydown="onKeydown"
      />
      <span class="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 text-xs">⌄</span>
    </div>
    <div
      v-if="isOpen"
      class="absolute z-20 mt-1 w-full bg-white border border-gray-200 rounded-md shadow-lg max-h-60 overflow-auto"
    >
      <button
        v-if="allowEmpty"
        type="button"
        class="w-full text-left px-3 py-2 text-sm hover:bg-gray-100 flex items-center justify-between"
        @mousedown.prevent
        @click="selectEmpty"
      >
        <span>{{ emptyLabel }}</span>
        <span v-if="String(modelValue) === ''" class="text-xs text-green-600">✔</span>
      </button>
      <template v-if="filteredOptions.length">
        <button
          v-for="opt in filteredOptions"
          :key="String(getValue(opt))"
          type="button"
          class="w-full text-left px-3 py-2 text-sm hover:bg-gray-100 flex items-start justify-between gap-2"
          @mousedown.prevent
          @click="selectOption(opt)"
        >
          <span>
            <div>{{ getLabel(opt) }}</div>
            <div v-if="getSubLabel(opt)" class="text-xs text-gray-500 mt-0.5">{{ getSubLabel(opt) }}</div>
          </span>
          <span v-if="isSelected(opt)" class="text-xs text-green-600 pt-0.5">✔</span>
        </button>
      </template>
      <div v-else class="px-3 py-2 text-xs text-gray-500">{{ emptyResultText }}</div>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps({
  modelValue: { type: [String, Number], default: '' },
  options: { type: Array, default: () => [] },
  valueKey: { type: String, default: 'value' },
  placeholder: { type: String, default: '请选择' },
  disabled: { type: Boolean, default: false },
  getLabel: { type: Function, default: (item) => item?.label ?? '' },
  getSubLabel: { type: Function, default: () => '' },
  allowEmpty: { type: Boolean, default: true },
  emptyLabel: { type: String, default: '不使用' },
  emptyResultText: { type: String, default: '未找到匹配项' },
})

const emit = defineEmits(['update:modelValue'])

const isOpen = ref(false)
const search = ref('')
const inputRef = ref(null)
const wrapperRef = ref(null)

const normalizedOptions = computed(() => {
  const key = props.valueKey
  return props.options.filter((opt) => opt && opt[key] !== undefined && opt[key] !== null)
})

const selectedOption = computed(() =>
  normalizedOptions.value.find((opt) => getValue(opt) === props.modelValue) || null,
)

const filteredOptions = computed(() => {
  const keyword = search.value.trim().toLowerCase()
  if (!keyword) return normalizedOptions.value
  const result = normalizedOptions.value.filter((opt) => {
    const label = (props.getLabel(opt) || '').toLowerCase()
    const sub = (props.getSubLabel(opt) || '').toLowerCase()
    return label.includes(keyword) || sub.includes(keyword)
  })
  if (
    selectedOption.value &&
    !result.some((opt) => getValue(opt) === getValue(selectedOption.value))
  ) {
    return [selectedOption.value, ...result]
  }
  return result
})

const displayValue = computed(() => {
  if (isOpen.value) {
    return search.value
  }
  if (String(props.modelValue) === '') {
    return ''
  }
  return selectedOption.value ? props.getLabel(selectedOption.value) : ''
})

function getValue(option) {
  return option?.[props.valueKey]
}

function isSelected(option) {
  return getValue(option) === props.modelValue
}

function openDropdown() {
  if (props.disabled) return
  if (!isOpen.value) {
    isOpen.value = true
    search.value = selectedOption.value ? props.getLabel(selectedOption.value) : ''
    nextTick(() => {
      inputRef.value?.focus()
      inputRef.value?.select()
    })
  }
}

function closeDropdown() {
  isOpen.value = false
  search.value = ''
}

function onInput(event) {
  if (!isOpen.value) {
    openDropdown()
  }
  search.value = event.target.value
}

function onKeydown(event) {
  if (event.key === 'Escape') {
    event.preventDefault()
    closeDropdown()
    inputRef.value?.blur()
  } else if (event.key === 'Enter') {
    event.preventDefault()
    if (!isOpen.value) {
      openDropdown()
      return
    }
    const first = filteredOptions.value[0]
    if (first) {
      selectOption(first)
    }
  } else if (event.key === 'ArrowDown' && !isOpen.value) {
    event.preventDefault()
    openDropdown()
  }
}

function selectOption(option) {
  emit('update:modelValue', getValue(option))
  closeDropdown()
  nextTick(() => {
    inputRef.value?.blur()
  })
}

function selectEmpty() {
  emit('update:modelValue', '')
  closeDropdown()
  nextTick(() => {
    inputRef.value?.blur()
  })
}

function handleClickOutside(event) {
  if (wrapperRef.value && !wrapperRef.value.contains(event.target)) {
    closeDropdown()
  }
}

onMounted(() => {
  document.addEventListener('mousedown', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', handleClickOutside)
})

watch(
  () => props.modelValue,
  () => {
    if (!isOpen.value) {
      search.value = ''
    }
  },
)
</script>
