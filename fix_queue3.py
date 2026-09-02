path = r'C:\Users\Lucas\Documents\GitHub\OpenPolyPrint\cmd\openpolyprint\main.go'
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

old = '\t' * 7 + 'if d := m.Find(s.ID); d != nil {\n' + \
'\t' * 8 + 'if err := d.StartPrint(ctx, next.Filename); err != nil {\n' + \
'\t' * 9 + 'log.Printf("[queue] auto-start %s on %s failed: %v", next.Filename, s.Name, err)\n' + \
'\t' * 9 + 'queueStore.UpdateStatus(next.ID, "failed", err.Error())\n' + \
'\t' * 8 + '} else {\n' + \
'\t' * 9 + 'log.Printf("[queue] auto-started %s on %s", next.Filename, s.Name)\n' + \
'\t' * 8 + '}\n' + \
'\t' * 7 + '}'

new = '\t' * 7 + 'if d := m.Find(s.ID); d != nil {\n' + \
'\t' * 8 + 'if err := uploadAndPrint(ctx, d, gcodeStore, next.Filename); err != nil {\n' + \
'\t' * 9 + 'log.Printf("[queue] auto-start %s on %s failed: %v", next.Filename, s.Name, err)\n' + \
'\t' * 9 + 'queueStore.UpdateStatus(next.ID, "failed", err.Error())\n' + \
'\t' * 8 + '} else {\n' + \
'\t' * 9 + 'log.Printf("[queue] auto-started %s on %s", next.Filename, s.Name)\n' + \
'\t' * 8 + '}\n' + \
'\t' * 7 + '}'

if old in content:
    content = content.replace(old, new, 1)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)
    print("OK: fixed auto queue start")
else:
    print("ERROR: pattern not found")
