<script lang="ts">
  import { goto } from '$app/navigation'
  import { base } from '$app/paths'
  import { onMount } from 'svelte'
  import Main from '$lib/layouts/Main.svelte'
  import { translate } from '$lib/i18n'

  let t = $derived($translate)

  onMount(() => {
    goto(`${base}/products`)
  })
</script>

<Main>
  <div>{t('common.redirecting')}</div>
</Main>
