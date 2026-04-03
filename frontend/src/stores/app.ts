import { defineStore } from 'pinia'

export const useAppStore = defineStore('app', {
  state: () => ({
    siderCollapsed: false,
  }),
  actions: {
    toggleSider() {
      this.siderCollapsed = !this.siderCollapsed
    },
  },
})