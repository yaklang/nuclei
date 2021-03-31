args1 = {
    'URL': URL,
    'param1': a, # External variable
    'param2': b  # External variable
}
res1 = run('one/template.yaml', args1)
run('another/template.yaml', res1)