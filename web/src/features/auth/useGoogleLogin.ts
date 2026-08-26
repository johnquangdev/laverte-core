import { useMutation } from '@tanstack/react-query'
import { fetchGoogleLoginUrl } from './api'

export function useGoogleLogin() {
  return useMutation({
    mutationFn: async () => {
      const url = await fetchGoogleLoginUrl()
      window.location.assign(url)
    },
  })
}
