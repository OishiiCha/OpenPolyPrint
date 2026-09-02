import re

path = r'C:\Users\Lucas\Documents\GitHub\OpenPolyPrint\cmd\openpolyprint\main.go'
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

# Fix 1: Manual queue start - replace d.StartPrint with uploadAndPrint
old1 = '''\t\t\tqueueStore.UpdateStatus(id, "printing", "")
\t\t\tif err := d.StartPrint(r.Context(), targetItem.Filename); err != nil {
\t\t\t\tqueueStore.UpdateStatus(id, "failed", err.Error())
\t\t\t\thttp.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
\t\t\t\treturn
\t\t\t}
\t\t\t_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})'''

new1 = '''\t\t\tqueueStore.UpdateStatus(id, "printing", "")
\t\t\tif err := uploadAndPrint(r.Context(), d, gcodeStore, targetItem.Filename); err != nil {
\t\t\t\tqueueStore.UpdateStatus(id, "failed", err.Error())
\t\t\t\thttp.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
\t\t\t\treturn
\t\t\t}
\t\t\t_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})'''

if old1 in content:
    content = content.replace(old1, new1, 1)
    print("OK: fixed manual queue start")
else:
    print("ERROR: manual start pattern not found")

# Fix 2: Auto queue start - replace d.StartPrint with uploadAndPrint
old2 = '''\t\t\t\t\t\tif d := m.Find(s.ID); d != nil {
\t\t\t\t\t\t\tif err := d.StartPrint(ctx, next.Filename); err != nil {
\t\t\t\t\t\t\t\tlog.Printf("[queue] auto-start %s on %s failed: %v", next.Filename, s.Name, err)
\t\t\t\t\t\t\t\tqueueStore.UpdateStatus(next.ID, "failed", err.Error())
\t\t\t\t\t\t\t} else {
\t\t\t\t\t\t\t\tlog.Printf("[queue] auto-started %s on %s", next.Filename, s.Name)
\t\t\t\t\t\t\t}
\t\t\t\t\t\t}'''

new2 = '''\t\t\t\t\t\tif d := m.Find(s.ID); d != nil {
\t\t\t\t\t\t\tif err := uploadAndPrint(ctx, d, gcodeStore, next.Filename); err != nil {
\t\t\t\t\t\t\t\tlog.Printf("[queue] auto-start %s on %s failed: %v", next.Filename, s.Name, err)
\t\t\t\t\t\t\t\tqueueStore.UpdateStatus(next.ID, "failed", err.Error())
\t\t\t\t\t\t\t} else {
\t\t\t\t\t\t\t\tlog.Printf("[queue] auto-started %s on %s", next.Filename, s.Name)
\t\t\t\t\t\t\t}
\t\t\t\t\t\t}'''

if old2 in content:
    content = content.replace(old2, new2, 1)
    print("OK: fixed auto queue start")
else:
    print("ERROR: auto start pattern not found")

with open(path, 'w', encoding='utf-8') as f:
    f.write(content)
print("DONE")
