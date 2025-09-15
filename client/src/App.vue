<template>
  <div class="max-w-5xl mx-auto p-6 space-y-8">
    <h1 class="text-2xl font-bold">胖哒哒数字人</h1>
    
    <!-- 全自动化处理 -->
    <section class="bg-green-50 border border-green-200 rounded-lg p-6">
      <h2 class="text-xl font-semibold text-green-800 mb-4">🚀 全自动化处理</h2>
      <p class="text-green-700 mb-4">只需上传音频和视频文件，系统将自动完成整个处理流程</p>
      
      <form @submit.prevent="startAutoProcess" class="space-y-4">
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-2">音频文件</label>
            <input type="file" accept="audio/*" @change="onAutoAudioPick" class="block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded-full file:border-0 file:text-sm file:font-semibold file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100" />
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-2">视频文件</label>
            <input type="file" accept="video/*" @change="onAutoVideoPick" class="block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded-full file:border-0 file:text-sm file:font-semibold file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100" />
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
        
        <button type="submit" :disabled="!autoAudioFile || !autoVideoFile || (autoUseTTS && !autoText)" 
                class="w-full bg-green-600 text-white py-3 px-4 rounded-lg font-medium hover:bg-green-700 disabled:bg-gray-300 disabled:cursor-not-allowed">
          开始全自动处理
        </button>
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
import { ref, onMounted } from 'vue'

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

function onAudioPick(e){ audioFile.value = e.target.files?.[0] }
function onVideoPick(e){ videoFile.value = e.target.files?.[0] }

function onAutoAudioPick(e){ autoAudioFile.value = e.target.files?.[0] }
function onAutoVideoPick(e){ autoVideoFile.value = e.target.files?.[0] }

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

// 自动化处理函数
async function startAutoProcess(){
  const fd = new FormData()
  fd.append('audio', autoAudioFile.value)
  fd.append('video', autoVideoFile.value)
  fd.append('speaker', autoSpeaker.value)
  fd.append('text', autoText.value)
  fd.append('copy_to_company', String(autoCopyToCompany.value))
  fd.append('use_tts', String(autoUseTTS.value))
  
  try {
    const r = await fetch('/api/auto/process', { method: 'POST', body: fd })
    const result = await r.json()
    
    if (result.task_id) {
      autoTaskId.value = result.task_id
      autoStatus.value = { status: 'processing', current_step: '开始处理', progress: 0 }
      
      // 开始轮询状态
      pollAutoStatus()
    } else {
      alert('启动自动化处理失败: ' + (result.error || '未知错误'))
    }
  } catch (error) {
    alert('启动自动化处理失败: ' + error.message)
  }
}

// 轮询自动化处理状态
async function pollAutoStatus(){
  if (!autoTaskId.value) return
  
  try {
    const r = await fetch(`/api/auto/status/${autoTaskId.value}`)
    const status = await r.json()
    
    autoStatus.value = status
    
    // 如果还在处理中，继续轮询
    if (status.status === 'processing') {
      setTimeout(pollAutoStatus, 3000) // 3秒轮询一次
    }
  } catch (error) {
    console.error('轮询状态失败:', error)
    setTimeout(pollAutoStatus, 5000) // 出错时5秒后重试
  }
}

// 格式化耗时显示
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

onMounted(refreshFiles)
</script>

<style scoped>
</style>
