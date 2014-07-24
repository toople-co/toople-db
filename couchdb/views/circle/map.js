function(doc) {
	if (doc.type == 'circle') {
		emit([doc._id, doc.type], null);
	}
	if (doc.type == 'member') {
		emit([doc.circle, 'user', doc.date], {_id: doc.user});
	}
	if (doc.type == 'event') {
		for (var i in doc.circles) {
			emit([i, doc.type, doc.date], null);
		}
	}
}