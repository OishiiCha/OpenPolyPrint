import re

path = r'C:\Users\Lucas\Documents\GitHub\OpenPolyPrint\cmd\openpolyprint\main.go'
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

old = '''\t\tcase http.MethodPost:
\t\t\t// Start a queued item manually
\t\t\tif d := mgr.Load().Find(id); d != nil {
\t\t\t\t// Find the queue item by looking through the list
\t\t\t\tfor _, item := range queueStore.List() {
\t\t\t\t\tif item.ID == id && item.Status == "pending" {
\t\t\t\t\t\tqueueStore.UpdateStatus(id, "printing", "")
\t\t\t\t\t\tif err := d.StartPrint(r.Context(), item.Filename); err != nil {
\t\t\t\t\t\t\tqueueStore.UpdateStatus(id, "failed", err.Error())
\t\t\t\t\t\t\thttp.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
\t\t\t\t\t\t\treturn
\t\t\t\t\t\t}
\t\t\t\t\t\t_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
\t\t\t\t\t\treturn
\t\t\t\t\t}
\t\t\t\t}
\t\t\t}
\t\t\thttp.Error(w, `{"error":"queue item not found or printer unavailable"}`, http.StatusNotFound)'''

new = '''\t\tcase http.MethodPost:
\t\t\t// Start a queued item manually
\t\t\t// First find the queue item to get its printerID
\t\t\tvar targetItem *queue.QueueItem
\t\t\tfor _, item := range queueStore.List() {
\t\t\t\tif item.ID == id && item.Status == "pending" {
\t\t\t\t\ttargetItem = &item
\t\t\t\t\tbreak
\t\t\t\t}
\t\t\t}
\t\t\tif targetItem == nil {
\t\t\t\thttp.Error(w, `{"error":"queue item not found or not pending"}`, http.StatusNotFound)
\t\t\t\treturn
\t\t\t}
\t\t\t// Find the printer using the queue item's printerID (not the queue item ID)
\t\t\td := mgr.Load().Find(targetItem.PrinterID)
\t\t\tif d == nil {
\t\t\t\tqueueStore.UpdateStatus(id, "failed", "printer not found: "+targetItem.PrinterID)
\t\t\t\thttp.Error(w, `{"error":"printer not found"}`, http.StatusNotFound)
\t\t\t\treturn
\t\t\t}
\t\t\tqueueStore.UpdateStatus(id, "printing", "")
\t\t\tif err := d.StartPrint(r.Context(), targetItem.Filename); err != nil {
\t\t\t\tqueueStore.UpdateStatus(id, "failed", err.Error())
\t\t\t\thttp.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
\t\t\t\treturn
\t\t\t}
\t\t\t_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})'''

if old in content:
    content = content.replace(old, new, 1)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)
    print("OK: replaced queue start handler")
else:
    print("ERROR: old string not found")
    # Try to find a partial match
    for i, line in enumerate(old.split('\n')):
        if line.strip() and line.strip() not in content:
            print(f"  Missing line: {repr(line)}")
            break
