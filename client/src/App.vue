<template>
  <div class="max-w-5xl mx-auto p-6 space-y-8">
    <h1 class="text-2xl font-bold">胖哒哒数字人</h1>
    
    <!-- 全自动化处理 -->
    <section class="bg-green-50 border border-green-200 rounded-lg p-6">
      <h2 class="text-xl font-semibold text-green-800 mb-4">🚀 全自动化处理</h2>
      <p class="text-green-700 mb-4">只需上传音频和视频文件，系统将自动完成整个处理流程</p>
      
      <form @submit.prevent="startAutoProcess" class="space-y-4">
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div class="space-y-3">
            <div>
              <label class="block text-sm font-medium text-gray-700 mb-2">音频文件</label>
              <input type="file" accept="audio/*" :disabled="!!selectedAudioTemplate" @change="onAutoAudioPick" class="block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded-full file:border-0 file:text-sm file:font-semibold file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100 disabled:opacity-60" />
              <p v-if="selectedAudioTemplate" class="mt-1 text-xs text-amber-600">已选择音频模版，若需重新上传请清空模版选择。</p>
            </div>
            <div class="rounded-lg border border-amber-200 bg-amber-50/60 p-3 space-y-2 text-sm text-gray-700">
              <div class="flex flex-wrap items-center justify-between gap-2">
                <span class="font-medium">音频模版库</span>
                <button type="button" class="px-2 py-1 text-xs rounded border border-amber-300 text-amber-700 bg-white hover:bg-amber-100" @click="fetchTemplates">刷新列表</button>
              </div>
              <div class="flex flex-wrap items-center gap-2 text-sm">
                <select v-model="selectedAudioTemplate" class="border rounded px-2 py-1 text-sm">
                  <option value="">不使用模版</option>
                  <option v-for="tpl in audioTemplates" :key="tpl.name" :value="tpl.name">
                    {{ tpl.display_name || tpl.name }}（更新 {{ formatTimestamp(tpl.updated_at) }}）
                  </option>
                </select>
                <span v-if="selectedAudioTemplateInfo" class="text-xs text-gray-500">已选：{{ selectedAudioTemplateInfo.display_name || selectedAudioTemplateInfo.name }}</span>
              </div>
              <div class="flex flex-wrap items-center gap-2 text-xs">
                <input v-model="newAudioTemplateName" placeholder="模版名称" class="border rounded px-2 py-1 text-xs flex-1 min-w-[10rem]" />
                <input type="file" accept="audio/*" :disabled="isUploadingAudioTemplate" @change="uploadAudioTemplate" class="block text-xs text-amber-700 file:mr-3 file:py-1.5 file:px-3 file:rounded-full file:border-0 file:bg-amber-100 hover:file:bg-amber-200 disabled:opacity-60" />
                <span v-if="audioTemplateMessage" :class="audioTemplateMessageType === 'error' ? 'text-red-600' : 'text-emerald-600'">{{ audioTemplateMessage }}</span>
              </div>
              <p class="text-xs text-amber-600">提示：模版音频会自动转换为 16k WAV，可任意命名并反复使用。</p>
            </div>
          </div>
          <div class="space-y-3">
            <div>
              <label class="block text-sm font-medium text-gray-700 mb-2">视频文件</label>
              <input type="file" accept="video/*" :disabled="!!selectedVideoTemplate" @change="onAutoVideoPick" class="block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded-full file:border-0 file:text-sm file:font-semibold file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100 disabled:opacity-60" />
              <p v-if="selectedVideoTemplate" class="mt-1 text-xs text-sky-600">已选择视频模版，若需重新上传请清空模版选择。</p>
            </div>
            <div class="rounded-lg border border-sky-200 bg-sky-50/60 p-3 space-y-2 text-sm text-gray-700">
              <div class="flex flex-wrap items-center justify-between gap-2">
                <span class="font-medium">视频模版库</span>
                <button type="button" class="px-2 py-1 text-xs rounded border border-sky-300 text-sky-700 bg-white hover:bg-sky-100" @click="fetchTemplates">刷新列表</button>
              </div>
              <div class="flex flex-wrap items-center gap-2 text-sm">
                <select v-model="selectedVideoTemplate" class="border rounded px-2 py-1 text-sm">
                  <option value="">不使用模版</option>
                  <option v-for="tpl in videoTemplates" :key="tpl.name" :value="tpl.name">
                    {{ tpl.display_name || tpl.name }}（更新 {{ formatTimestamp(tpl.updated_at) }}）
                  </option>
                </select>
                <span v-if="selectedVideoTemplateInfo" class="text-xs text-gray-500">已选：{{ selectedVideoTemplateInfo.display_name || selectedVideoTemplateInfo.name }}</span>
              </div>
              <div class="flex flex-wrap items-center gap-2 text-xs">
                <input v-model="newVideoTemplateName" placeholder="模版名称" class="border rounded px-2 py-1 text-xs flex-1 min-w-[10rem]" />
                <input type="file" accept="video/*" :disabled="isUploadingVideoTemplate" @change="uploadVideoTemplate" class="block text-xs text-sky-700 file:mr-3 file:py-1.5 file:px-3 file:rounded-full file:border-0 file:bg-sky-100 hover:file:bg-sky-200 disabled:opacity-60" />
                <span v-if="videoTemplateMessage" :class="videoTemplateMessageType === 'error' ? 'text-red-600' : 'text-emerald-600'">{{ videoTemplateMessage }}</span>
              </div>
              <p class="text-xs text-sky-600">提示：视频模版会自动转为 MP4，适合存放不同人物形象。</p>
            </div>
          </div>
        </div>
        
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-2">说话人ID</label>
            <input v-model="autoSpeaker" placeholder="demo001" class="w-full border rounded px-3 py-2" />
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-2">要合成的文本</label>
            <textarea v-model="autoText" :disabled="!autoUseTTS" placeholder="当使用自带音频时可留空" class="w-full border rounded px-3 py-2 h-20"></textarea>
          </div>
        </div>
        
        <div class="flex items-center gap-4">
          <label class="flex items-center">
            <input type="checkbox" v-model="autoUseTTS" class="mr-2" />
            <span class="text-sm">使用 TTS 合成（关闭则使用自带音频）</span>
          </label>
          <label class="flex items-center">
            <input type="checkbox" v-model="autoCopyToCompany" class="mr-2" />
            <span class="text-sm">拷贝到Windows目录</span>
          </label>
        </div>
        
        <button type="submit" :disabled="autoSubmitDisabled" 
                class="w-full bg-green-600 text-white py-3 px-4 rounded-lg font-medium hover:bg-green-700 disabled:bg-gray-300 disabled:cursor-not-allowed">
          开始全自动处理
        </button>

        <!-- 上传进度条 -->
        <div v-if="isUploading" class="mt-4 space-y-2">
          <div class="text-sm text-gray-600">正在上传音/视频到服务器，请稍候...</div>
          <div>
            <div class="flex justify-between text-xs text-gray-600"><span>音频</span><span>{{ uploadAudioPercent }}%</span></div>
            <div class="w-full bg-gray-200 rounded h-2">
              <div class="bg-blue-600 h-2 rounded" :style="{ width: uploadAudioPercent + '%' }"></div>
            </div>
          </div>
          <div>
            <div class="flex justify-between text-xs text-gray-600"><span>视频</span><span>{{ uploadVideoPercent }}%</span></div>
            <div class="w-full bg-gray-200 rounded h-2">
              <div class="bg-blue-600 h-2 rounded" :style="{ width: uploadVideoPercent + '%' }"></div>
            </div>
          </div>
          <div>
            <div class="flex justify-between text-xs text-gray-600"><span>总进度</span><span>{{ uploadPercent }}%</span></div>
            <div class="w-full bg-gray-200 rounded h-2">
              <div class="bg-green-600 h-2 rounded" :style="{ width: uploadPercent + '%' }"></div>
            </div>
          </div>
          <div v-if="uploadError" class="text-red-600 text-sm">上传失败：{{ uploadError }}</div>
        </div>
      </form>
      
      <!-- 自动化处理状态 -->
      <div v-if="autoStatus" class="mt-6 p-4 bg-white rounded-lg border">
        <h3 class="font-semibold mb-2">处理状态</h3>
        <div class="space-y-2">
          <div class="flex justify-between items-center">
            <span class="text-sm font-medium">{{ autoStatus.current_step }}</span>
            <span class="text-sm text-gray-600">{{ autoStatus.progress }}%</span>
          </div>
          <div class="w-full bg-gray-200 rounded-full h-2">
            <div class="bg-green-600 h-2 rounded-full transition-all duration-300" :style="{ width: autoStatus.progress + '%' }"></div>
          </div>
          <div v-if="autoStatus.status === 'completed'" class="text-green-600 font-medium">
            ✅ 处理完成！视频文件：{{ autoStatus.result_video }}
            <div class="text-sm text-gray-600 mt-1">
              总耗时：{{ formatDuration(autoStatus.total_duration) }}
            </div>
            <div class="mt-2">
              <a :href="`/api/download/video/${autoStatus.result_video}`" 
                 class="inline-block bg-green-600 text-white px-4 py-2 rounded hover:bg-green-700 transition-colors">
                📥 下载视频
              </a>
            </div>
          </div>
          <div v-if="autoStatus.status === 'failed'" class="text-red-600 font-medium">
            ❌ 处理失败：{{ autoStatus.error }}
          </div>
        </div>
      </div>
    </section>

    <!-- 任务队列与批量下载 -->
    <section class="bg-white p-4 rounded shadow space-y-3">
      <div class="flex items-center justify-between">
        <h2 class="font-semibold">📋 任务队列</h2>
        <div class="space-x-2">
          <button class="px-3 py-1 bg-gray-100 rounded border" @click="refreshTasks">刷新</button>
          <button class="px-3 py-1 bg-purple-600 text-white rounded" :disabled="selectedTaskIds.length===0" @click="downloadSelected">打包下载选中</button>
          <a class="px-3 py-1 bg-purple-600 text-white rounded" href="/api/auto/archive?all=1">打包下载全部已完成</a>
        </div>
      </div>
      <div class="overflow-x-auto">
        <table class="min-w-full text-sm">
          <thead>
            <tr class="text-left border-b">
              <th class="p-2">选择</th>
              <th class="p-2">任务ID</th>
              <th class="p-2">状态</th>
              <th class="p-2">进度</th>
              <th class="p-2">当前步骤</th>
              <th class="p-2">耗时</th>
              <th class="p-2">错误</th>
              <th class="p-2">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="t in taskList" :key="t.task_id" class="border-b">
              <td class="p-2">
                <input type="checkbox" :disabled="t.status !== 'completed'" :value="t.task_id" v-model="selectedTaskIds" />
              </td>
              <td class="p-2 whitespace-nowrap">{{ t.task_id }}</td>
              <td class="p-2">{{ t.status }}</td>
              <td class="p-2">{{ t.progress }}%</td>
              <td class="p-2">{{ t.current_step }}</td>
              <td class="p-2">{{ t.total_duration ? formatDuration(t.total_duration) : (t.start_time ? '进行中' : '-') }}</td>
              <td class="p-2 text-red-600 max-w-[20ch] truncate" :title="t.error">{{ t.error }}</td>
              <td class="p-2">
                <a v-if="t.status==='completed'" :href="`/api/download/video/${t.result_video}`" class="text-blue-600 hover:underline">下载</a>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section class="bg-white p-4 rounded shadow space-y-3">
      <h2 class="font-semibold">1) 音频上传与标准化</h2>
      <form @submit.prevent="uploadAudio" class="flex items-center gap-3 flex-wrap">
        <input type="file" accept="audio/*" @change="onAudioPick" class="block" />
        <button class="px-3 py-1 bg-blue-600 text-white rounded" :disabled="!audioFile">开始处理</button>
      </form>
      <div v-if="audioResult" class="text-sm text-slate-600">
        <div>参考音频: {{ audioResult.reference_audio }}</div>
        <div>已拷贝: {{ audioResult.copied_to }}</div>
      </div>
    </section>

    <section class="bg-white p-4 rounded shadow space-y-3">
      <h2 class="font-semibold">2) 视频上传并静音</h2>
      <form @submit.prevent="uploadVideo" class="flex items-center gap-3 flex-wrap">
        <input type="file" accept="video/*" @change="onVideoPick" class="block" />
        <button class="px-3 py-1 bg-blue-600 text-white rounded" :disabled="!videoFile">生成静音视频</button>
      </form>
      <div v-if="videoResult" class="text-sm text-slate-600">已生成: {{ videoResult.copied_to }}</div>
    </section>

    <section class="bg-white p-4 rounded shadow space-y-3">
      <h2 class="font-semibold">3) 语音预处理 + TTS 合成</h2>
      <div class="flex items-center gap-2 flex-wrap">
        <button class="px-3 py-1 bg-emerald-600 text-white rounded" @click="preprocess">调用预处理</button>
        <span class="text-sm text-slate-600" v-if="preResp.reference_audio_text">ASR 文本: {{ preResp.reference_audio_text }}</span>
      </div>
      <div class="flex items-center gap-2 flex-wrap">
        <input class="border rounded px-2 py-1 w-64" placeholder="Speaker (默认 demo001)" v-model="speaker" />
        <input class="border rounded px-2 py-1 flex-1" placeholder="合成文本" v-model="ttsText" />
        <button class="px-3 py-1 bg-emerald-600 text-white rounded" @click="invokeTTS">合成 TTS</button>
      </div>
      <div v-if="ttsOut" class="text-sm text-slate-600">TTS 已保存并复制到视频目录: {{ ttsOut.copied_to_video_dir }}</div>
    </section>

    <section class="bg-white p-4 rounded shadow space-y-3">
      <h2 class="font-semibold">4) 提交视频合成任务</h2>
      <div class="flex gap-2 flex-wrap items-center">
        <select v-model="selAudio" class="border rounded px-2 py-1">
          <option disabled value="">选择音频(视频目录)</option>
          <option v-for="f in files.video" :key="f" :value="f">{{ f }}</option>
        </select>
        <select v-model="selVideo" class="border rounded px-2 py-1">
          <option disabled value="">选择视频(视频目录)</option>
          <option v-for="f in files.video" :key="'v-' + f" :value="f">{{ f }}</option>
        </select>
        <input class="border rounded px-2 py-1" placeholder="任务 code (task001)" v-model="taskCode" />
        <button class="px-3 py-1 bg-purple-600 text-white rounded" @click="submitVideo">提交任务</button>
        <button class="px-3 py-1 bg-slate-700 text-white rounded" @click="refreshFiles">刷新文件</button>
      </div>
      <div v-if="submitResp" class="text-sm text-slate-600">已提交: {{ submitResp.upstream_status }} {{ submitResp.upstream_body }}</div>
    </section>

    <section class="bg-white p-4 rounded shadow space-y-3">
      <h2 class="font-semibold">5) 拉取结果并复制</h2>
      <div class="flex items-center gap-2 flex-wrap">
        <input class="border rounded px-2 py-1" placeholder="任务 code (task001)" v-model="resultCode" />
        <label class="inline-flex items-center gap-2"><input type="checkbox" v-model="copyToCompany" /> 复制到 /mnt/c/company</label>
        <button class="px-3 py-1 bg-teal-600 text-white rounded" @click="fetchResult">拉取结果</button>
      </div>
      <div v-if="resultResp" class="text-sm text-slate-600">结果: {{ resultResp.result }} <span v-if="resultResp.copied_to_company"> => {{ resultResp.copied_to_company }}</span></div>
    </section>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, watch, computed } from 'vue'

const audioFile = ref(null)
const videoFile = ref(null)
const trimSilence = ref(true)
const audioResult = ref(null)
const videoResult = ref(null)

const preResp = ref({})
const speaker = ref('demo001')
const ttsText = ref('')
const ttsOut = ref(null)

const files = ref({ video: [] })
const selAudio = ref('')
const selVideo = ref('')
const taskCode = ref('task001')
const submitResp = ref(null)

const resultCode = ref('task001')
const copyToCompany = ref(false)
const resultResp = ref(null)

// 自动化处理相关
const autoAudioFile = ref(null)
const autoVideoFile = ref(null)
const autoSpeaker = ref('demo001')
const autoText = ref('')
const autoCopyToCompany = ref(false)
const autoUseTTS = ref(true)
const autoStatus = ref(null)
const autoTaskId = ref('')

const audioTemplates = ref([])
const videoTemplates = ref([])
const selectedAudioTemplate = ref('')
const selectedVideoTemplate = ref('')
const newAudioTemplateName = ref('')
const newVideoTemplateName = ref('')
const isUploadingAudioTemplate = ref(false)
const isUploadingVideoTemplate = ref(false)
const audioTemplateMessage = ref('')
const videoTemplateMessage = ref('')
const audioTemplateMessageType = ref('success')
const videoTemplateMessageType = ref('success')

// 上传进度
const isUploading = ref(false)
const uploadPercent = ref(0)
const uploadAudioPercent = ref(0)
const uploadVideoPercent = ref(0)
const uploadError = ref('')

const autoSubmitDisabled = computed(() => {
  if (isUploading.value) return true
  const hasAudioSource = !!selectedAudioTemplate.value || !!autoAudioFile.value
  const hasVideoSource = !!selectedVideoTemplate.value || !!autoVideoFile.value
  if (!hasAudioSource) return true
  if (!hasVideoSource) return true
  if (autoUseTTS.value && !autoText.value) return true
  return false
})

watch(selectedAudioTemplate, (val) => {
  if (val) {
    autoAudioFile.value = null
  }
})

watch(autoAudioFile, (file) => {
  if (file) {
    selectedAudioTemplate.value = ''
  }
})

watch(selectedVideoTemplate, (val) => {
  if (val) {
    autoVideoFile.value = null
  }
})

watch(autoVideoFile, (file) => {
  if (file) {
    selectedVideoTemplate.value = ''
  }
})

const selectedAudioTemplateInfo = computed(() => audioTemplates.value.find(t => t.name === selectedAudioTemplate.value) || null)
const selectedVideoTemplateInfo = computed(() => videoTemplates.value.find(t => t.name === selectedVideoTemplate.value) || null)

// 队列列表和批量下载
const taskList = ref([])
const selectedTaskIds = ref([])
let tasksTimer = null

function onAudioPick(e){ audioFile.value = e.target.files?.[0] }
function onVideoPick(e){ videoFile.value = e.target.files?.[0] }

function onAutoAudioPick(e){
  autoAudioFile.value = e.target.files?.[0]
  if (autoAudioFile.value) {
    selectedAudioTemplate.value = ''
  }
}
function onAutoVideoPick(e){
  autoVideoFile.value = e.target.files?.[0]
  if (autoVideoFile.value) {
    selectedVideoTemplate.value = ''
  }
}

async function uploadAudioTemplate(e){
  const file = e.target.files?.[0]
  if (!file) return
  if (!newAudioTemplateName.value.trim()) {
    audioTemplateMessage.value = '请先填写模版名称'
    audioTemplateMessageType.value = 'error'
    e.target.value = ''
    return
  }
  audioTemplateMessage.value = ''
  audioTemplateMessageType.value = 'success'
  isUploadingAudioTemplate.value = true
  const fd = new FormData()
  fd.append('name', newAudioTemplateName.value.trim())
  fd.append('file', file)
  try {
    const resp = await fetch('/api/templates/audio', { method: 'POST', body: fd })
    const data = await resp.json().catch(() => ({}))
    if (!resp.ok || data.error) {
      throw new Error(data.error || `HTTP ${resp.status}`)
    }
    await fetchTemplates()
    const tpl = data.template
    if (tpl?.name) {
      selectedAudioTemplate.value = tpl.name
    }
    audioTemplateMessage.value = '音频模版上传成功'
    audioTemplateMessageType.value = 'success'
    autoAudioFile.value = null
    newAudioTemplateName.value = ''
  } catch (err) {
    console.error('音频模版上传失败', err)
    audioTemplateMessage.value = `音频模版上传失败：${err?.message || err}`
    audioTemplateMessageType.value = 'error'
  } finally {
    isUploadingAudioTemplate.value = false
    e.target.value = ''
  }
}

async function uploadVideoTemplate(e){
  const file = e.target.files?.[0]
  if (!file) return
  if (!newVideoTemplateName.value.trim()) {
    videoTemplateMessage.value = '请先填写模版名称'
    videoTemplateMessageType.value = 'error'
    e.target.value = ''
    return
  }
  videoTemplateMessage.value = ''
  videoTemplateMessageType.value = 'success'
  isUploadingVideoTemplate.value = true
  const fd = new FormData()
  fd.append('name', newVideoTemplateName.value.trim())
  fd.append('file', file)
  try {
    const resp = await fetch('/api/templates/video', { method: 'POST', body: fd })
    const data = await resp.json().catch(() => ({}))
    if (!resp.ok || data.error) {
      throw new Error(data.error || `HTTP ${resp.status}`)
    }
    await fetchTemplates()
    const tpl = data.template
    if (tpl?.name) {
      selectedVideoTemplate.value = tpl.name
    }
    videoTemplateMessage.value = '视频模版上传成功'
    videoTemplateMessageType.value = 'success'
    autoVideoFile.value = null
    newVideoTemplateName.value = ''
  } catch (err) {
    console.error('视频模版上传失败', err)
    videoTemplateMessage.value = `视频模版上传失败：${err?.message || err}`
    videoTemplateMessageType.value = 'error'
  } finally {
    isUploadingVideoTemplate.value = false
    e.target.value = ''
  }
}

async function uploadAudio(){
  const fd = new FormData()
  fd.append('file', audioFile.value)
  const r = await fetch('/api/upload/audio', { method: 'POST', body: fd })
  audioResult.value = await r.json()
}

async function uploadVideo(){
  const fd = new FormData()
  fd.append('file', videoFile.value)
  const r = await fetch('/api/upload/video', { method: 'POST', body: fd })
  videoResult.value = await r.json()
  await refreshFiles()
}

async function preprocess(){
  const r = await fetch('/api/tts/preprocess', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ format: 'wav', reference_audio: 'ref_norm.wav', lang: 'zh' })
  })
  preResp.value = await r.json()
}

async function invokeTTS(){
  const payload = {
    speaker: speaker.value,
    text: ttsText.value,
    format: 'wav',
    topP: 0.7,
    max_new_tokens: 1024,
    chunk_length: 100,
    repetition_penalty: 1.2,
    temperature: 0.7,
    need_asr: false,
    streaming: false,
    is_fixed_seed: 0,
    is_norm: 0,
    reference_audio: preResp.value.asr_format_audio_url || '',
    reference_text: preResp.value.reference_audio_text || ''
  }
  const r = await fetch('/api/tts/invoke', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) })
  ttsOut.value = await r.json()
  await refreshFiles()
}

async function submitVideo(){
  const payload = { audio_filename: selAudio.value, video_filename: selVideo.value, code: taskCode.value, pn: 1, chaofen: 0, watermark_switch: 0 }
  const r = await fetch('/api/video/submit', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) })
  submitResp.value = await r.json()
}

async function fetchResult(){
  const r = await fetch(`/api/video/result?code=${encodeURIComponent(resultCode.value)}&copy_to_company=${copyToCompany.value ? '1' : '0'}`)
  resultResp.value = await r.json()
}

async function refreshFiles(){
  const r = await fetch('/api/files?dir=video')
  const j = await r.json()
  files.value.video = j.files || []
  if(!selVideo.value && files.value.video.includes('silent.mp4')) selVideo.value = 'silent.mp4'
  if(!selAudio.value) selAudio.value = files.value.video.find(f => f.endsWith('.wav')) || ''
}

async function fetchTemplates(){
  try {
    const resp = await fetch('/api/templates')
    const data = await resp.json()
    audioTemplates.value = Array.isArray(data.audio) ? data.audio : []
    videoTemplates.value = Array.isArray(data.video) ? data.video : []
    if (selectedAudioTemplate.value && !audioTemplates.value.some(t => t.name === selectedAudioTemplate.value)) {
      selectedAudioTemplate.value = ''
    }
    if (selectedVideoTemplate.value && !videoTemplates.value.some(t => t.name === selectedVideoTemplate.value)) {
      selectedVideoTemplate.value = ''
    }
  } catch (err) {
    console.error('获取模版信息失败', err)
  }
}

// 自动化处理函数
async function startAutoProcess(){
  const usingAudioTemplate = !!selectedAudioTemplate.value
  const usingVideoTemplate = !!selectedVideoTemplate.value
  if (!usingAudioTemplate && !autoAudioFile.value) {
    audioTemplateMessage.value = '请上传音频文件或选择模版'
    audioTemplateMessageType.value = 'error'
    return
  }
  if (!usingVideoTemplate && !autoVideoFile.value) {
    videoTemplateMessage.value = '请上传视频文件或选择模版'
    videoTemplateMessageType.value = 'error'
    return
  }

  const fd = new FormData()
  if (usingAudioTemplate) {
    fd.append('audio_template_name', selectedAudioTemplate.value)
  } else if (autoAudioFile.value) {
    fd.append('audio', autoAudioFile.value)
  }
  if (usingVideoTemplate) {
    fd.append('video_template_name', selectedVideoTemplate.value)
  } else if (autoVideoFile.value) {
    fd.append('video', autoVideoFile.value)
  }
  fd.append('speaker', autoSpeaker.value)
  fd.append('text', autoText.value)
  fd.append('copy_to_company', String(autoCopyToCompany.value))
  fd.append('use_tts', String(autoUseTTS.value))

  // 使用 XHR 以便拿到上传进度
  isUploading.value = true
  uploadPercent.value = (usingAudioTemplate && usingVideoTemplate) ? 100 : 0
  uploadAudioPercent.value = usingAudioTemplate ? 100 : 0
  uploadVideoPercent.value = usingVideoTemplate ? 100 : 0
  uploadError.value = ''

  const audioSize = usingAudioTemplate ? 0 : (autoAudioFile.value?.size || 0)
  const videoSize = usingVideoTemplate ? 0 : (autoVideoFile.value?.size || 0)

  const xhr = new XMLHttpRequest()
  xhr.open('POST', '/api/auto/process', true)

  let lastTs = Date.now()
  let lastLoaded = 0
  xhr.upload.onprogress = (e) => {
    if (!e.lengthComputable) return
    const loaded = e.loaded
    const total = e.total
    uploadPercent.value = Math.min(100, Math.round((loaded / total) * 100))
    // 估算分摊到两个文件的进度：音频在前、视频在后（近似）
    const audioLoaded = Math.min(loaded, audioSize)
    const videoLoaded = Math.max(0, loaded - audioSize)
    uploadAudioPercent.value = audioSize ? Math.min(100, Math.round((audioLoaded / audioSize) * 100)) : 100
    uploadVideoPercent.value = videoSize ? Math.min(100, Math.round((Math.min(videoLoaded, videoSize) / videoSize) * 100)) : 100

    // 简易速度（可选）
    const now = Date.now()
    const dt = (now - lastTs) / 1000
    if (dt >= 0.5) {
      const speed = (loaded - lastLoaded) / dt // bytes/s
      lastTs = now
      lastLoaded = loaded
      // 可在此扩展显示速度/剩余时间（当前不显示）
    }
  }

  xhr.onerror = () => {
    isUploading.value = false
    uploadError.value = '网络错误，请重试'
  }
  xhr.onload = () => {
    isUploading.value = false
    try {
      const result = JSON.parse(xhr.responseText || '{}')
      if (xhr.status >= 400 || result.error) {
        uploadError.value = result.error || `服务错误(${xhr.status})`
        return
      }
      if (result.task_id) {
        autoTaskId.value = result.task_id
        autoStatus.value = { status: 'queued', current_step: '等待排队执行', progress: 0 }
        pollAutoStatus()
      } else {
        uploadError.value = '未返回任务ID'
      }
    } catch (err) {
      uploadError.value = '解析返回失败'
    }
  }

  xhr.send(fd)
}

// 轮询自动化处理状态
async function pollAutoStatus(){
  if (!autoTaskId.value) return
  
  try {
    const r = await fetch(`/api/auto/status/${autoTaskId.value}`)
    const status = await r.json()
    
    autoStatus.value = status
    
    // 如果还未结束，继续轮询（含 queued/processing）
    if (status.status !== 'completed' && status.status !== 'failed') {
      setTimeout(pollAutoStatus, 3000) // 3秒轮询一次
    }
  } catch (error) {
    console.error('轮询状态失败:', error)
    setTimeout(pollAutoStatus, 5000) // 出错时5秒后重试
  }
}

// 格式化耗时显示
function formatTimestamp(ts) {
  if (!ts) return '未更新'
  const d = new Date(ts * 1000)
  if (Number.isNaN(d.getTime())) return '未更新'
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function formatDuration(seconds) {
  if (!seconds) return '计算中...'
  
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const secs = seconds % 60
  
  if (hours > 0) {
    return `${hours}小时${minutes}分钟${secs}秒`
  } else if (minutes > 0) {
    return `${minutes}分钟${secs}秒`
  } else {
    return `${secs}秒`
  }
}

onMounted(() => {
  refreshFiles()
  fetchTemplates()
  refreshTasks()
  tasksTimer = setInterval(refreshTasks, 5000)
})
onUnmounted(() => { if (tasksTimer) clearInterval(tasksTimer) })

async function refreshTasks(){
  try {
    const r = await fetch('/api/auto/tasks')
    const j = await r.json()
    taskList.value = j.tasks || []
  } catch (e) {
    console.error('刷新队列失败', e)
  }
}

function downloadSelected(){
  if (selectedTaskIds.value.length === 0) return
  const q = encodeURIComponent(selectedTaskIds.value.join(','))
  window.location.href = `/api/auto/archive?task_ids=${q}`
}

onMounted(refreshTasks)
</script>

<style scoped>
</style>
