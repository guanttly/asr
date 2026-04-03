import { defineStore } from 'pinia'

const MAX_LIVE_SENTENCES = 500

export const useTranscriptionStore = defineStore('transcription', {
  state: () => ({
    isRecording: false,
    liveSentences: [] as string[],
    draftText: '',
    transcriptText: '',
    totalSentenceCount: 0,
  }),
  actions: {
    appendSentence(sentence: string) {
      const normalized = sentence.trim()
      if (!normalized)
        return

      this.liveSentences.push(sentence)
      if (this.liveSentences.length > MAX_LIVE_SENTENCES)
        this.liveSentences.splice(0, this.liveSentences.length - MAX_LIVE_SENTENCES)

      this.transcriptText = this.transcriptText
        ? `${this.transcriptText}\n${normalized}`
        : normalized
      this.totalSentenceCount += 1
    },
    setDraftText(text: string) {
      this.draftText = text
    },
    reset() {
      this.liveSentences = []
      this.draftText = ''
      this.transcriptText = ''
      this.totalSentenceCount = 0
      this.isRecording = false
    },
  },
})