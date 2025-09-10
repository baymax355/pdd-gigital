<template>
  <div class="max-w-5xl mx-auto p-6 space-y-8">
    <h1 class="text-2xl font-bold">HeyGem 全流程助手</h1>

    <section class="bg-white p-4 rounded shadow space-y-3">
      <h2 class="font-semibold">1) 音频上传与标准化</h2>
      <form @submit.prevent="uploadAudio" class="flex items-center gap-3 flex-wrap">
        <input type="file" accept="audio/*" @change="onAudioPick" class="block" />
        <label class="inline-flex items-center gap-2">
          <input type="checkbox" v-model="trimSilence" /> 去头尾静音
        </label>
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

function onAudioPick(e){ audioFile.value = e.target.files?.[0] }
function onVideoPick(e){ videoFile.value = e.target.files?.[0] }

async function uploadAudio(){
  const fd = new FormData()
  fd.append('file', audioFile.value)
  fd.append('trim_silence', String(trimSilence.value))
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

onMounted(refreshFiles)
</script>

<style scoped>
</style>

