import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import OAuthAuthorizationFlow from '../OAuthAuthorizationFlow.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('OAuthAuthorizationFlow Claude authorization methods', () => {
  it('offers credentials.json import without showing Cookie auto authorization for OAuth', () => {
    const wrapper = mount(OAuthAuthorizationFlow, {
      props: {
        addMethod: 'oauth',
        platform: 'anthropic',
        showCookieOption: false,
        showCredentialsImportOption: true
      },
      global: { plugins: [createPinia()], stubs: { Icon: true } }
    })

    expect(wrapper.text()).toContain('admin.accounts.oauth.credentialsImport')
    expect(wrapper.text()).not.toContain('admin.accounts.oauth.cookieAutoAuth')
  })

  it('keeps Cookie authorization available for setup-token flows only', () => {
    const wrapper = mount(OAuthAuthorizationFlow, {
      props: {
        addMethod: 'setup-token',
        platform: 'anthropic',
        showCookieOption: true,
        showCredentialsImportOption: false
      },
      global: { plugins: [createPinia()], stubs: { Icon: true } }
    })

    expect(wrapper.text()).toContain('admin.accounts.oauth.cookieAutoAuth')
    expect(wrapper.text()).not.toContain('admin.accounts.oauth.credentialsImport')
  })
})
