import { defineStore } from 'pinia'

import { getCurrentUser } from '@/api/user'

type UserProfile = {
  id: number
  username: string
  displayName: string
  role: string
}

export const useUserStore = defineStore('user', {
  state: () => ({
    token: localStorage.getItem('asr_token') || '',
    profile: null as UserProfile | null,
    ready: false,
  }),
  actions: {
    setToken(token: string) {
      this.token = token
      localStorage.setItem('asr_token', token)
    },
    setProfile(profile: UserProfile | null) {
      this.profile = profile
    },
    logout() {
      this.token = ''
      this.profile = null
      this.ready = true
      localStorage.removeItem('asr_token')
    },
    async bootstrap() {
      if (!this.token) {
        this.ready = true
        return
      }

      try {
        const result = await getCurrentUser()
        this.setProfile(result.data)
      }
      catch {
        this.logout()
      }
      finally {
        this.ready = true
      }
    },
  },
})