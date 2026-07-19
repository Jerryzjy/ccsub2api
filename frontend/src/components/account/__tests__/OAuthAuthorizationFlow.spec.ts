import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import OAuthAuthorizationFlow from '../OAuthAuthorizationFlow.vue'

const createAccountModalPath = resolve(
  dirname(fileURLToPath(import.meta.url)),
  '../CreateAccountModal.vue'
)
const createAccountModalSource = readFileSync(createAccountModalPath, 'utf8')

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('OAuthAuthorizationFlow Claude authorization methods', () => {
  it('offers sessionKey authorization and credentials.json import together for OAuth', () => {
    const wrapper = mount(OAuthAuthorizationFlow, {
      props: {
        addMethod: 'oauth',
        platform: 'anthropic',
        showCookieOption: true,
        showCredentialsImportOption: true
      },
      global: { plugins: [createPinia()], stubs: { Icon: true } }
    })

    expect(wrapper.text()).toContain('admin.accounts.oauth.credentialsImport')
    expect(wrapper.text()).toContain('admin.accounts.oauth.cookieAutoAuth')
  })

  it('enables the sessionKey option from the Claude OAuth account creation parent', () => {
    expect(createAccountModalSource).toContain(':show-cookie-option="form.platform === \'anthropic\'"')
    expect(createAccountModalSource).toContain("? '/admin/accounts/cookie-auth'")
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
