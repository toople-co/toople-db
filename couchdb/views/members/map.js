function(doc) {
	if (doc.type == 'member') {
		emit([doc.circle, doc.date], {_id: doc.user});
	}
}
