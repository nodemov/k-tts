<script lang="ts">
	interface Props {
		label: string;
		checked: boolean;
		onchange?: (v: boolean) => void;
	}

	let { label, checked = $bindable(), onchange }: Props = $props();
</script>

<button
	type="button"
	class="btn btn-sm justify-start"
	aria-pressed={checked}
	onclick={() => {
		checked = !checked;
		onchange?.(checked);
	}}
>
	<span class="led" class:on={checked}></span>
	{label}
</button>
