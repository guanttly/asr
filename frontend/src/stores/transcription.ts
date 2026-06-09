import { defineStore } from 'pinia'

const MAX_LIVE_SENTENCES = 500

export const useTranscriptionStore = defineStore('transcription', {
  state: () => ({
    isRecording: false,
    liveSentences: [] as string[],
    processedSentences: [] as string[],
    processedSentenceBase: 0,
    draftText: '',
    transcriptText: '',
    processedTranscriptText: '',
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
    appendProcessedSentence(sentence: string) {
      const normalized = sentence.trim()
      if (!normalized)
        return

      this.processedSentences.push(normalized)
      if (this.processedSentences.length > MAX_LIVE_SENTENCES) {
        const removed = this.processedSentences.length - MAX_LIVE_SENTENCES
        this.processedSentences.splice(0, removed)
        this.processedSentenceBase += removed
      }

      this.processedTranscriptText = this.processedTranscriptText
        ? `${this.processedTranscriptText}\n${normalized}`
        : normalized
    },
    setDraftText(text: string) {
      this.draftText = text
    },
    reset() {
      this.liveSentences = []
      this.processedSentences = []
      this.processedSentenceBase = 0
      this.draftText = ''
      this.transcriptText = ''
      this.processedTranscriptText = ''
      this.totalSentenceCount = 0
      this.isRecording = false
    },
  },
})
