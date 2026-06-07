<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import FormButton from '../form/Button.svelte'
  import FormInput from '../form/Input.svelte'
  import FormToggle from '../form/Toggle.svelte'
  import { loadPaymentSettings, savePaymentSettings, togglePaymentActive } from '$lib/composables/usePaymentSettings'
  import { systemStore } from '$lib/stores/system'
  import { translate } from '$lib/i18n'

  // Reactive translation function
  let t = $derived($translate)

  interface Props {
    onclose?: () => void
  }

  let { onclose }: Props = $props()

  interface BepusdtSettings {
    active: boolean
    api_token: string
    api_url: string
  }

  let settings = $state<BepusdtSettings>({
    active: false,
    api_token: '',
    api_url: ''
  })
  let formErrors = $state<Record<string, string>>({})
  let unsubscribe: (() => void) | null = null

  onMount(async () => {
    settings = await loadPaymentSettings<BepusdtSettings>('bepusdt', settings)

    unsubscribe = systemStore.subscribe((store) => {
      if (store.payments?.bepusdt !== undefined) {
        settings.active = store.payments.bepusdt
      }
    })
  })

  onDestroy(() => {
    unsubscribe?.()
  })

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault()
    formErrors = {}

    if (!settings.api_token || settings.api_token.length < 8) {
      formErrors.api_token = t('validation.minLength').replace('{{min}}', '8')
      return
    }

    if (!settings.api_url) {
      formErrors.api_url = t('validation.required')
      return
    }

    await savePaymentSettings('bepusdt', settings)
  }

  async function handleToggleActive() {
    const previousValue = settings.active
    const success = await togglePaymentActive('bepusdt', settings.active)

    if (!success) {
      settings.active = previousValue
    }
  }

  function close() {
    onclose?.()
  }
</script>

<div>
  <div class="pb-8">
    <div class="flex items-center">
      <div class="pr-3">
        <h1>BEpusdt (USDT)</h1>
      </div>
      <div class="pt-1">
        <FormToggle
          id="bepusdt-active"
          bind:value={settings.active}
          disabled={Object.keys(formErrors).length > 0}
          onchange={handleToggleActive}
        />
      </div>
    </div>
  </div>

  <form onsubmit={handleSubmit}>
    <div class="flow-root">
      <dl class="mx-auto -my-3 mt-2 mb-0 space-y-4 text-sm">
        <FormInput
          id="api_token"
          type="text"
          title={t('payment.apiToken')}
          bind:value={settings.api_token}
          error={formErrors.api_token}
          ico="key"
        />
        <FormInput
          id="api_url"
          type="text"
          title={t('payment.apiUrl')}
          bind:value={settings.api_url}
          error={formErrors.api_url}
          ico="glob-alt"
        />
      </dl>
    </div>

    <div class="pt-8">
      <div class="flex">
        <div class="flex-none">
          <FormButton type="submit" name={t('common.save')} color="green" />
        </div>
        <div class="grow"></div>
        <div class="flex-none">
          <FormButton type="button" name={t('common.close')} color="gray" onclick={close} />
        </div>
      </div>
    </div>
  </form>
</div>
